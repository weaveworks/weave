# AWSVPC

## Today

- Change container type
- Fix breakage with DOCKER_BRIDGE_IP

- Extend the 900 tests that it would create container on the same host
- Test stop/launch <- does it trigger update? does it have to trigger update?
- Check destination in the route assertions
- Rebase master
- Rebase master

* Cleanup the code
- Get rid of monitor status
* Add the docs
* Change alloc.universe type to address.CIDR
* Rename useAWSVPC -> isAWSVPC

- Add tests for proxy
* Add tests for plugin

---> Disable multiple subnets (iprangecidr - universe, ipsubnetcidr - default?)
* Check what happens when we use default subnet which is larger than the universe
* Check that ipalloc-range does not overlap with the host subnet
* What happens if $RCIDR is not found

* Do fuzz testing

* Get rid of ethtool tx off for AWSVPC

- Fix `weave status`
- Use NewNull()
- Fix `weave report` re "no bridge networking"

* Read on VPC routing tables again.

* Check how calico does the networking
- Check how flannel does the networking
* Check how swarm does the networking

- There is a bug when `weave reset` does not clean the rt properly <= probably
  because of weave stop exec upon end_suite
- weave stop -> weave reset does not cleanup the tables, does it notify other
  IPAM peers?

- Check why the first packet takes ages to send && maybe check with gw setup
* Check why there are not so many packets on ARP
* Check whether flooding increases with proxy_delay--
* What happens when we have thousands containers on the same host, does
  flooding bites us?

* Benchmark to see whether proxy_arp sux.
- What happens when we run fastdp and awsvpc at the same time.
* Enable --awsvpc in other tests
* Test re-attach of containers
* Bring $HOST3

## aws.sh

* do not delete keys
* create a separate VPC for each run
* AWS=1 might be set for GCE machines, add the reset cmd

## Misc

Invalid log msg:
DEBU: 2016/05/27 10:01:15.116349 [allocator 62:5e:2b:83:97:6e]: Allocated
10.32.0.1 for weave:expose in 10.32.0.0/12
DEBU: 2016/05/27 10:01:15.528758 [http] GET /status
DEBU: 2016/05/27 10:01:15.547612 [http] PUT
/ip/weave:expose/10.32.0.1/12?noErrorOnUnknown=true
DEBU: 2016/05/27 10:01:15.547754 [allocator 62:5e:2b:83:97:6e]: Re-Claimed
10.32.0.1/12 for weave:expose
