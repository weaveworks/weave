---
title: Troubleshooting
layout: default
---

The command

    weave status

reports on the current status of various weave components, including
the proxy, if it is running:

````
...
weave proxy is running
````

Information on the operation of the proxy can be obtained from the
weaveproxy container logs using:

    docker logs weaveproxy

**See Also**

 * [Troubleshooting Weave](/site/troubleshooting.md)