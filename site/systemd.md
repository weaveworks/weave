---
title: Using Weave with systemd
layout: default
---

Having installed `weave` as per [readme][], you might wish to configure the
init daemon to start it on boot. Most recent Linux distribution releases are
shipping with [systemd][]. The information below should provide you with some
initial guidance on getting a weave service configured on systemd-based OS.

## Weave Service Unit and Configuration

A regular service unit definition for weave is shown below and you should
normally place it in `/etc/systemd/system/weave.service`.

    [Unit]
    Description=Weave Network
    Documentation=http://docs.weave.works/weave/latest_release/
    Requires=docker.service
    After=docker.service
    [Service]
    EnvironmentFile=-/etc/sysconfig/weave
    ExecStartPre=/usr/local/bin/weave launch $PEERS
    ExecStart=/usr/bin/docker logs -f weave
    SuccessExitStatus=2
    ExecStop=/usr/local/bin/weave stop
    [Install]
    WantedBy=multi-user.target


To specify the addresses or names of other weave hosts to join the network
you can create the `/etc/sysconfig/weave` environment file which would be of
the following format:

    PEERS="HOST1 HOST2 .. HOSTn"

You can also use the [connect][] command to add participating hosts dynamically.

Additionally, if you want to enable [encryption][] you can specify a password with
`WEAVE_PASSWORD="MakeSureThisIsSecure"` in the `/etc/sysconfig/weave` environment
file, and it will get picked up by weave on launch.

You now should be able to launch weave with

    sudo systemctl start weave

To ensure weave launches after reboot, you need run

    sudo systemctl enable weave

For more information on systemd, please refer to the documentation supplied
by your distribution of Linux.

## SELinux Tweaks

If your OS has SELinux enabled and you wish to run weave as a systemd unit,
then you should follow the instructions below. These instructions apply to
CentOS and RHEL as of 7.0. On Fedora 21, there is no need to do this.

Once you have installed `weave` in `/usr/local/bin`, set its execution
context with the commands shown below. You will need to have the
`policycoreutils-python` package installed.

    sudo semanage fcontext -a -t unconfined_exec_t -f f /usr/local/bin/weave
    sudo restorecon /usr/local/bin/weave

[readme]: https://github.com/weaveworks/weave/blob/master/README.md#installation
[connect]: features.html#dynamic-topologies
[systemd]: http://www.freedesktop.org/wiki/Software/systemd/
[encryption]: how-it-works.html#crypto
