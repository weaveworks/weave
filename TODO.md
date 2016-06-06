# AWSVPC

# Urgent

* Add tests for rmpeer
* Check whether reset is not async

* Add the subnet checks / keep checksum offloading for the plugin
* Test with the plugin

* Simplify / get rid of some tests
* Document ipam/tracker/awsvpc.go

* Add $HOST3 to awsvpc.sh
* Test with multiple addr from the default subnet

* Create new SSH keys or reuse the same for each test run

# Nice To Have

* What happens if $RCIDR is not found
* Do fuzz testing
* Read on VPC routing tables again.
* create a separate VPC for each test run
