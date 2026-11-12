#!/bin/bash
# Runs the end-to-end scripts in the container: the use cases UC01-UC13, the advanced tools UC14-UC17 and the features of 0.2.0
# UC19-UC23 (their results are added to the same usecases.tsv).
cd "$(dirname "$0")"
./e2e.sh; a=$?
APPEND=1 ./e2e-tools.sh; b=$?
APPEND=1 ./e2e-new.sh; c=$?
[ $a -eq 0 ] && [ $b -eq 0 ] && [ $c -eq 0 ]
