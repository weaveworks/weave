<!--
Hi, thank you for opening an issue!
Before hitting the button...

** Is this a REQUEST FOR HELP? **
If so, please have a look at:
- our FAQ: https://www.weave.works/docs/net/latest/faq/
- our troubleshooting page: https://www.weave.works/docs/net/latest/troubleshooting/
- our help page, to choose the best channel (Slack, etc.) to reach out: https://www.weave.works/help/

** Is this a FEATURE REQUEST? **
If so, please search existing feature requests, and if you find a similar one, up-vote it and/or add your comments to it instead.
If you did not find a similar one, please describe in details:
- why: your use-case, specific constraints you may have, etc.
- what: the feature/behaviour/change you would like to see in Weave Net
Do not hesitate, when appropriate, to share the exact commands or API you would like, and/or to share a diagram (e.g.: asciiflow.com): "a picture is worth a thousand words".

** Is this a BUG REPORT? **
Please fill in as much of the template below as you can.

Thank you!
-->

## What you expected to happen?

## What happened?
<!-- Error message, actual behaviour, etc. -->

## How to reproduce it?
<!-- Specific steps, as minimally and precisely as possible. -->

## Anything else we need to know?
<!-- Cloud provider? Hardware? How did you configure your cluster? Kubernetes YAML, KOPS, etc. -->

## Versions:
<!-- Please paste in the output of these commands; 'kubectl' only if using Kubernetes -->
```
$ weave version
$ docker version
$ uname -a
$ kubectl version
```

## Logs:
```
$ docker logs weave
```
or, if using Kubernetes:
```
$ kubectl logs -n kube-system <weave-net-pod> weave
```
<!-- (If output is long, please consider a Gist.) -->
<!-- Anything interesting or unusual output by the below, potentially relevant, commands?
$ journalctl -u docker.service --no-pager
$ journalctl -u kubelet --no-pager
$ kubectl get events
-->

## Network:
<!-- If your problem has anything to do with one network endpoint not being able to contact another, please run the following commands -->
```
$ ip route
$ ip -4 -o addr
$ sudo iptables-save
```
