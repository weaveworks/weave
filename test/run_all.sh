#!/bin/bash

. ./config.sh

whitely echo Sanity checks
if ! bash ./sanity_check.sh; then
    whitely echo ...failed
    exit 1
fi
whitely echo ...ok

# Modified version of _assert_cleanup from assert.sh that
# prints overall status
check_test_status() {
    if [ $? -ne 0 ]; then
        redly echo "---= !!!ABNORMAL TEST TERMINATION!!! =---"
    elif [ $tests_suite_status -ne 0 ]; then
        redly echo "---= !!!SUITE FAILURES - SEE ABOVE FOR DETAILS!!! =---"
        exit $tests_suite_status
    else
        greenly echo "---= ALL SUITES PASSED =---"
    fi
}
# Overwrite assert.sh _assert_cleanup trap with our own
trap check_test_status EXIT

TESTS="${@:-*_test.sh}"

# If running on circle, use the scheduler to work out what tests to run
if [ -n "$CIRCLECI" ]; then
    TESTS=$(echo $TESTS | ./sched sched $CIRCLE_BUILD_NUM $CIRCLE_NODE_TOTAL $CIRCLE_NODE_INDEX)
fi

echo Running $TESTS

for t in $TESTS; do
    echo
    greyly echo "---= Running $t =---"
    . $t

    # Report test runtime when running on circle, to help scheduler
    if [ -n "$CIRCLECI" ]; then
        ./sched time $t $(bc -l <<< "$tests_time/1000000000")
    fi
done
