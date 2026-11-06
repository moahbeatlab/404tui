#!/bin/bash
# Runs both end-to-end scripts in the container: the use cases UC01-UC13 and the advanced tools UC14-UC17 (their results are added
# to the same usecases.tsv).
cd "$(dirname "$0")"
./e2e.sh; a=$?
APPEND=1 ./e2e-tools.sh; b=$?
[ $a -eq 0 ] && [ $b -eq 0 ]
