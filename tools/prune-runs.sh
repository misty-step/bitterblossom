#!/bin/sh
# Prune old run artifacts under .bb/runs.
DAYS=$1
RUNS_DIR=".bb/runs"

# delete artifacts older than N days
find $RUNS_DIR -type d -mtime +$DAYS | while read dir; do
  rm -rf $dir
done

echo "pruned runs older than $DAYS days"
