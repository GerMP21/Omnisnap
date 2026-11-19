"""Train the cause classifier (gradient-boosted trees, LightGBM).

Data: out/dataset.csv from dataset.mjs. Label 1 = a cause produced by a bug
injected into a real page (mutation testing), label 0 = a natural cause of a
clean page (engine differences; unlabeled, mostly expected; positive-
unlabeled). Precision on this table depends on how many bugs were injected,
so it is not reported as a result. The metrics that transfer are RECALL on
injected bugs and FALSE ALARMS on natural causes: the share reported and
Report causes per clean page pair. Methods are compared at equal recall.

Evaluation never sees a site twice: GroupKFold by site, out-of-fold scores.
Reported next to the rules tier on the same rows. The threshold tau is the
largest score that keeps out-of-fold recall >= TARGET_RECALL.

Outputs (models/):
  gbdt.txt        LightGBM native model (Booster.model_to_string)
  gbdt.json       Booster.dump_model(): read by lib/gbdt.mjs and, later, Go
  gbdt-meta.json  features, tau, cross-validated metrics, importances
  parity.csv      rows + LightGBM predictions, for the JS evaluator's parity test

usage: python tools/train.py --data <dataset dir> [--models assets/model]
"""

import argparse
import json
import os
from datetime import date

import lightgbm as lgb
import numpy as np
import pandas as pd
from sklearn.metrics import precision_recall_curve, roc_auc_score
from sklearn.model_selection import GroupKFold

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))  # repo root
TARGET_RECALL = 0.9
PARAMS = {
    "objective": "binary",
    "learning_rate": 0.05,
    "num_leaves": 15,
    "min_data_in_leaf": 10,
    "feature_fraction": 0.8,
    "bagging_fraction": 0.8,
    "bagging_freq": 1,
    "lambda_l2": 1.0,
    "verbose": -1,
    "seed": 7,
    "deterministic": True,
}
ROUNDS = 300


def precision_at_recall(y, s, recall):
    """Precision and threshold at the largest threshold whose recall >= `recall`."""
    p, r, t = precision_recall_curve(y, s)
    ok = np.where(r[:-1] >= recall)[0]
    if not len(ok):
        return float("nan"), float("nan")
    i = ok[-1]
    return float(p[i]), float(t[i])


def wilson(k, n, z=1.96):
    if n == 0:
        return (0.0, 1.0)
    p = k / n
    den = 1 + z * z / n
    mid = (p + z * z / (2 * n)) / den
    half = z * np.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / den
    return (max(0.0, mid - half), min(1.0, mid + half))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--data", default=os.environ.get("OMNISNAP_OUT", os.path.join(HERE, "out")),
                    help="directory with dataset.csv and dataset-meta.json")
    ap.add_argument("--models", default=os.path.join(HERE, "assets", "model"),
                    help="where gbdt.txt/gbdt.json/gbdt-meta.json go")
    ap.add_argument("--parity", default=os.path.join(HERE, "testdata", "model", "parity.json"),
                    help="where the Go parity fixture goes (LightGBM's predictions on real rows)")
    ap.add_argument("--scores", default="",
                    help="where the out-of-fold score of every row goes (site,id,label,score)")
    ap.add_argument("--holdout", default="",
                    help="file with the page slugs held out of the corpus table; their false alarms are reported apart")
    args = ap.parse_args()
    df = pd.read_csv(os.path.join(args.data, "dataset.csv"))
    # Rows with a negative label are not training rows: planted pages have
    # their own gate (the planted audit), and a held-out set that leaks into
    # training is not held out.
    held_out = int((df["label"] < 0).sum())
    if held_out:
        df = df[df["label"] >= 0].reset_index(drop=True)
        print(f"dropped {held_out} row(s) with a negative label (not training rows)")
    feats = [c for c in df.columns if c not in ("site", "page", "pair", "group", "id", "label")]
    X = df[feats].to_numpy(dtype=np.float64)
    y = df["label"].to_numpy()
    sites = df["site"].to_numpy()
    n_pos, n_neg = int(y.sum()), int((1 - y).sum())
    print(f"rows {len(df)}: {n_pos} positive, {n_neg} negative, {len(set(sites))} sites, {len(feats)} features")
    params = {**PARAMS, "scale_pos_weight": max(1.0, n_neg / max(1, n_pos))}

    # ---- out-of-fold scores, grouped by site
    folds = min(5, len(set(sites)))
    oof = np.zeros(len(df))
    for k, (tr, te) in enumerate(GroupKFold(n_splits=folds).split(X, y, sites)):
        bst = lgb.train(params, lgb.Dataset(X[tr], y[tr], feature_name=feats), num_boost_round=ROUNDS)
        oof[te] = bst.predict(X[te])
    auc = roc_auc_score(y, oof)
    meta_in = json.load(open(os.path.join(args.data, "dataset-meta.json")))
    pairs = max(1, meta_in["cleanPairs"])
    rules = df["rules_report"].to_numpy() == 1

    def point(rep):
        tp, fp = int((rep & (y == 1)).sum()), int((rep & (y == 0)).sum())
        return {"recall": tp / max(1, n_pos), "recall_ci": wilson(tp, n_pos), "tp": tp,
                "false_alarm_rate": fp / max(1, n_neg), "fp": fp, "reports_per_clean_page": fp / pairs}

    def show(name, pt, extra=""):
        lo, hi = pt["recall_ci"]
        print(f"  {name:<34} recall {pt['tp']}/{n_pos} = {pt['recall']:.1%} [{lo:.1%}, {hi:.1%}]   natural causes reported {pt['fp']}/{n_neg} = {pt['false_alarm_rate']:.1%}   Report causes per clean page {pt['reports_per_clean_page']:.2f}{extra}")

    print(f"cross-validated (GroupKFold by site, {folds} folds): ROC AUC {auc:.3f}; {pairs} clean page pairs")
    if args.scores:
        out = df.assign(score=oof)[["site", "page", "pair", "group", "id", "label", "score"]]
        out.to_csv(args.scores, index=False)
        print(f"wrote the out-of-fold score of every row to {args.scores}")
    r_pt = point(rules)
    show("rules tier", r_pt)
    # Trees at the rules' recall: same bugs found, how many fewer false alarms?
    _, tau_rules = precision_at_recall(y, oof, r_pt["recall"])
    t_rules = point(oof >= tau_rules)
    show("trees at the rules' recall", t_rules, f"   (tau {tau_rules:.3f})")
    _, tau = precision_at_recall(y, oof, TARGET_RECALL)
    t_pt = point(oof >= tau)
    show(f"trees at recall >= {TARGET_RECALL:.0%}", t_pt, f"   (tau {tau:.3f})")
    rep = oof >= tau
    if args.holdout:
        held = set(l.strip() for l in open(args.holdout) if l.strip())
        m = df["site"].isin(held)
        if m.any():
            rep_h = oof[m] >= tau
            rules_h = df.loc[m, "rules_report"].to_numpy() == 1
            y_h = df.loc[m, "label"].to_numpy()
            npos_h = int((y_h == 1).sum())
            nneg_h = int((y_h == 0).sum())
            print(f"  held-out pages ({int(m.sum())} rows, {df.loc[m, 'site'].nunique()} sites): "
                  f"rules report {int((rules_h & (y_h == 0)).sum())}/{nneg_h} natural cause(s); "
                  f"trees report {int((rep_h & (y_h == 0)).sum())}/{nneg_h}")
    per_group = df.assign(score=oof, rep=rep).groupby("group").agg(rows=("label", "size"), reported=("rep", "sum"))
    print(per_group.to_string())

    # ---- final model on everything
    final = lgb.train(params, lgb.Dataset(X, y, feature_name=feats), num_boost_round=ROUNDS)
    models = args.models
    os.makedirs(models, exist_ok=True)
    with open(os.path.join(models, "gbdt.txt"), "w") as fh:
        fh.write(final.model_to_string())
    with open(os.path.join(models, "gbdt.json"), "w") as fh:
        json.dump(final.dump_model(), fh)
    gain = final.feature_importance(importance_type="gain")
    top = sorted(zip(feats, gain.tolist()), key=lambda x: -x[1])[:20]
    meta = {
        "trained": str(date.today()),
        "lightgbm": lgb.__version__,
        "features": feats,
        "tau": tau,
        "tau_at_rules_recall": tau_rules,
        "target_recall": TARGET_RECALL,
        "rows": {"positive": n_pos, "negative": n_neg, "sites": len(set(sites))},
        "cv": {"folds": folds, "roc_auc": auc, "clean_pairs": pairs, "rules": r_pt, "trees_at_rules_recall": t_rules, "trees_at_target": t_pt},
        "importance_gain_top20": top,
        "note": "label 0 rows are unlabeled natural causes (positive-unlabeled); compare recall and false alarms, not precision",
    }
    with open(os.path.join(models, "gbdt-meta.json"), "w") as fh:
        json.dump(meta, fh, indent=1)
    # Parity rows: the JS evaluator must reproduce these predictions.
    sample = df.sample(n=min(400, len(df)), random_state=1)
    sample.assign(pred=final.predict(sample[feats].to_numpy(dtype=np.float64)))[
        ["pred", *feats]
    ].to_csv(os.path.join(args.data, "parity.csv"), index=False)
    # The Go parity fixture: rows with the feature vector, the identity of the
    # cause and LightGBM's own prediction, so the Go evaluator is checked
    # against LightGBM and not against itself. It stays small on purpose: 30
    # rows are enough to catch an evaluator that diverges, and the file is
    # read in diffs.
    fixture = df.sample(n=min(30, len(df)), random_state=2)
    parity_rows = []
    for _, row in fixture.iterrows():
        parity_rows.append({
            "slug": row["page"],
            "engine": row["pair"],
            "id": row["id"],
            "p": float(final.predict(np.array([[row[f] for f in feats]], dtype=np.float64))[0]),
            "x": [float(row[f]) for f in feats],
        })
    os.makedirs(os.path.dirname(args.parity), exist_ok=True)
    with open(args.parity, "w") as fh:
        json.dump({"features": feats, "rows": parity_rows}, fh, indent=1)
    print("top features by gain: " + ", ".join(f"{n} {g:.0f}" for n, g in top[:10]))
    print(f"wrote {models}/gbdt.txt, gbdt.json, gbdt-meta.json; parity rows in "
          f"{os.path.join(args.data, 'parity.csv')} and {args.parity}")


if __name__ == "__main__":
    main()
