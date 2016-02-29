---
title: Plugin Command-line Arguments
layout: default
---



If you need to give additional arguments to the plugin independently, don't
use `weave launch`, but instead run:

    $ weave launch-router [other peers]
    $ weave launch-plugin [plugin arguments]

The plugin command-line arguments are:

 * `--log-level=debug|info|warning|error` --tells the plugin
   how much information to emit for debugging.
 * `--mesh-network-name=<name>` -- set <name> to blank to disable creation
   of a default network, or include a name of your own choice.
 * `--no-multicast-route` -- stops weave from adding a static IP route for
   multicast traffic onto its interface

By default, multicast traffic is routed over the weave network.
To turn this off, e.g. you want to configure your own multicast
route, add the `--no-multicast-route` flag to `weave launch-plugin`.


>>Note: When using the Docker Plugin, there is no need to run eval $(weave env) to enable the Proxy. Because Weave is running as a plugin within Docker, the Weave Docker API Proxy, at present, cannot detect between networks.  

**See Also**

 * [Using the Weave Net Docker Network Plugin](/site/plugin/weave-plugin-how-to.md)
 * [How the Weave Network Plugin Works](/site/plugin/plugin-how-it-works.md)