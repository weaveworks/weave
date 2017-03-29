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
If so, please search existing bug reports, and if you find a similar one, up-vote it and/or add your comments to it instead.
If you did not find a similar one, please fill in as much of the template below as you can.
If you leave out information, we will not be able to help you as well.

In all cases, be ready for follow-up questions and please respond in a timely manner.
If we cannot reproduce a bug or think a feature already exists, we might close your issue.
If we are wrong, please feel free to reopen it and explain why.

Thank you!
-->

## Environment:

### Weave:
```
$ weave report
```

### Docker:
```
$ docker version
$ docker-runc --version
```

### Kubernetes:
```
$ kubectl version
```

### Logs:
<!-- Anything interesting or unusual output by the below, potentially relevant, commands? -->
<!-- (If output is long, please consider a Gist.) -->
```
$ docker logs weave
$ journalctl -u docker.service --no-pager
$ kubectl logs -n kube-system <weave-net-pod> weave
$ kubectl get events
```

### OS:
```
$ uname -a
$ cat /etc/*release*
```

### Network:
```
$ ip route
$ ip -4 -o addr
```
<!-- Anything interesting or unusual output by iptables-save? -->
<!-- (If output is long, please consider a Gist.) -->

## What you expected to happen?

## What happened?
<!-- Error message, actual behaviour, etc. -->

## How to reproduce it?
<!-- Specific steps, as minimally and precisely as possible. -->

## Anything else we need to know?
<!-- Cloud provider? Hardware? How did you configure your cluster? Kubernetes YAML, KOPS, etc. -->
