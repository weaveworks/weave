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

for t in *_test.sh; do
    echo
    greyly echo "---= Running $t =---"
    . $t
done
