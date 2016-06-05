# AWSVPC

# Urgent

* Add the subnet checks / keep checksum offloading for the plugin
* Test with the plugin

* weave rmpeer should delete vpc routing entries for the peer if they still exist (we need peer->instanceid mapping)
* AdminTakeoverRanges remove vpc routing entries if heir does not exist

* Simplify / get rid of some tests
* Document ipam/tracker/awsvpc.go

* Add $HOST3 to awsvpc.sh
* Test with multiple addr from the default subnet

* Create new SSH keys or reuse the same for each test run
* Make sure that the $AWS=1 env var is not propagated to GCE machines

# Nice To Have

* What happens if $RCIDR is not found
* Do fuzz testing
* Read on VPC routing tables again.
* create a separate VPC for each test run
