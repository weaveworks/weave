---
title: Troubleshooting
layout: default
---

The command

~~~bash
    weave status
~~~

reports on the current status of various Weave Net components, including
the proxy, if it is running:

~~~bash
weave proxy is running
~~~

Information on the operation of the proxy can be obtained from the
weaveproxy container logs using:

~~~bash
    docker logs weaveproxy
~~~

**See Also**

 * [Troubleshooting Weave](/site/troubleshooting.md)