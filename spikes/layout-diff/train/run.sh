#!/usr/bin/env bash
# Full training run for the tree model, on a Linux disk:
#   1. planted suite (independent test, threshold calibration)
#   2. own + real + crawl pages (hand-picked + crawl-sites.json), $SHARDS shards
#   3. mutants on every clean page (MUT_PER_ENGINE per engine), $SHARDS shards
#   4. diff -> dataset -> train -> parity -> score
# Pages already captured are skipped (--resume), so an interrupted run
# continues where it stopped; FRESH=1 recaptures everything.
# SHARDS=1 runs one capture process (less memory, about twice as slow).
# CRAWL_LIMIT=n caps the crawl at n sites (captured ones first, then by rank).
# Logs: $OMNISNAP_OUT/logs/.
#
# usage: train/run.sh [step...]   steps: planted pages mut analyze (default: all)
set -euo pipefail
cd "$(dirname "$0")/.."
export OMNISNAP_OUT="${OMNISNAP_OUT:-$HOME/omnisnap-data/out}"
export MUT_PER_ENGINE="${MUT_PER_ENGINE:-4}"
PY="${PY:-$HOME/.venvs/omnisnap-spike/bin/python}"
E=chrome,firefox,webkit
SHARDS="${SHARDS:-2}"
R=$([ "${FRESH:-0}" = 1 ] || echo --resume)
# Run a capture group in $SHARDS parallel processes.
sharded() { local g=$1; shift; local i; for ((i = 0; i < SHARDS; i++)); do node capture.mjs "$g" "$@" $R --shard "$i/$SHARDS" > "$LOGS/$g-$i.log" 2>&1 & done; wait; }
LOGS="$OMNISNAP_OUT/logs"
mkdir -p "$LOGS"
steps=("$@")
[ ${#steps[@]} -eq 0 ] && steps=(planted pages mut analyze)
has() { [[ " ${steps[*]} " == *" $1 "* ]]; }
stamp() { date '+%F %T'; }

if has planted; then
  echo "$(stamp) planted"
  node capture.mjs planted --engines $E $R > "$LOGS/planted.log" 2>&1
fi
if has pages; then
  echo "$(stamp) own + real"
  node capture.mjs own --engines $E $R > "$LOGS/own.log" 2>&1
  sharded real --engines $E
  echo "$(stamp) crawl ($SHARDS shard(s))"
  sharded crawl --engines $E
fi
if has mut; then
  echo "$(stamp) mutants ($SHARDS shard(s), $MUT_PER_ENGINE per engine)"
  sharded mut --engines firefox,webkit
fi
if has analyze; then
  echo "$(stamp) diff"
  node --max-old-space-size=6000 diff.mjs > "$LOGS/diff.log" 2>&1
  echo "$(stamp) dataset + train + parity"
  node dataset.mjs | tee "$LOGS/dataset.log"
  "$PY" train/train.py | tee "$LOGS/train.log"
  node train/parity.mjs | tee "$LOGS/parity.log"
  echo "$(stamp) rescore with the model"
  node --max-old-space-size=6000 diff.mjs > "$LOGS/diff-model.log" 2>&1
  node score.mjs > "$LOGS/score.log" 2>&1
fi
echo "$(stamp) done"
