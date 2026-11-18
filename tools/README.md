# Lab tools

`train.py` trains the cause classifier from a dataset exported by
`omnisnap-lab dataset`: gradient-boosted trees, GroupKFold by site, fixed seed
(7) and fixed rounds, so the same dataset gives the same model bytes with the
pinned LightGBM version.

```
python -m venv .venv && . .venv/bin/activate
pip install -r tools/requirements.txt
python tools/train.py --data lab-out/dataset --models assets/model
go test ./internal/triage/ -run ModelParity
```

Outputs: `assets/model/gbdt.json` (evaluated by `internal/triage`),
`gbdt.txt` (LightGBM native, for other tools), `gbdt-meta.json` (features, tau,
CV metrics) and `parity.csv` in the dataset directory. A dataset row's label is
1 for an injected bug, 0 for a natural cause of a clean page; compare recall and
false alarms, not precision.
