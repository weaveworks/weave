#!/usr/bin/env python

import sys
import argparse
import logging
from collections import namedtuple
from fnmatch import fnmatch
from urlparse import urljoin
from urllib import urlopen
import csv

try:
    import plotly
    import plotly.plotly as py
    from plotly.graph_objs import *
except ImportError:
    print "FATAL: you need to install plotly (try 'pip install plotly'"
    sys.exit(1)
    

try:
    import requests
except ImportError:
    print "FATAL: you need to install requests (try 'pip install requests'"
    sys.exit(1)


API_ROOT    = "https://circleci.com/api/v1"
USERNAME    = "weaveworks"
PROJECT     = "weave"
BRANCH      = "master"
TOKEN       = None
BUILDS_NUM  = 10

#######################################################################################

def get_latest_builds(options, allow_failures=False):
    filt = 'completed' if allow_failures else 'successful'
    target = '{root}/project/{user}/{project}/tree/{branch}?limit={num}&filter={filt}'.format(
                root=options.api_root,
                user=options.username,
                project=options.project,
                branch=options.branch,
                num=options.num_builds,
                filt=filt)
    if options.token is not None:
        target += '&circle-token={}'.format(options.token)
    data = requests.get(target, headers={'Accept': 'application/json'})
    output = data.json()
    if len(output) == 0:
        raise ValueError('No matching builds.')
    return [x['build_num'] for x in output]

def get_artifact_list(options, build):
    target = '{root}/project/{user}/{project}/{build}/artifacts'.format(
                root=API_ROOT,
                user=USERNAME,
                project=PROJECT,
                build=build)
    if options.token is not None:
        target += '?circle-token={}'.format(options.token)
    data = requests.get(target, headers={'Accept': 'application/json'})
    output = data.json()
    return {x['pretty_path'][18:]: x['url'] for x in output}


# iperf CSV output example:
# 20140114124826,127.0.0.1,54402,127.0.0.1,5001,5,0.0-10.0,52551090176,42041052917
# 20140114124826,127.0.0.1,5001,127.0.0.1,54402,4,0.0-10.0,52551090200,41999020136

StatsLine = namedtuple('StatsLine',
                       ['timestamp',
                        'orig_ip',
                        'orig_port',
                        'dst_ip',
                        'dst_port',
                        'unknown',
                        'time',
                        'data',
                        'throughput'])

BuildStats = namedtuple('BuildStats',
                       ['num',
                        'file'])
                       
# parse a CSV log filename, returning a list of stats lines
# @param: a read()'able file or data
def parse_log(data):
    return [StatsLine._make(line) for line in csv.reader(data)]

# get an average throughtput for a  list of lines
def average_throughtput(stat_lines):
    return sum([int(l.throughput) for l in stat_lines])/len(stat_lines)

def average_throughtput_mbps(stat_lines):
    return average_throughtput(stat_lines) / (1024 * 1024)

if __name__ == "__main__":
    arguments = argparse.ArgumentParser(description="A CircleCI performance reports plotter")
    arguments.add_argument('--api-root', type=str,
                           help='API root for CircleCI', default=API_ROOT)
    arguments.add_argument('--token', type=str,
                           help='API key', nargs='?')
    arguments.add_argument('--username', type=str,
                           help='GitHub username', default=USERNAME)
    arguments.add_argument('--project', type=str,
                           help='GitHub project', default=PROJECT)
    arguments.add_argument('--branch', type=str,
                           help='Project branch', default=BRANCH)
    arguments.add_argument('--plot-username', type=str, dest='plot_username',
                           help='Ploty username', required=True)
    arguments.add_argument('--plot-key', type=str, dest='plot_key',
                           help='Ploty API key', required=True)
    arguments.add_argument('--plot-name', type=str, dest='plot_name',
                           help='Ploty plot name', default=None)
    arguments.add_argument('--plot-description', type=str, dest='plot_desc',
                           help='Ploty plot description', default="Throughput")
    arguments.add_argument('--plot-save-to', dest='plot_save_to',
                           help='Save the Ploty plot to a local PNG file',
                           type=argparse.FileType('w'), default=None)
    arguments.add_argument('--num', type=int, dest='num_builds',
                           default=BUILDS_NUM, help='Number of builds to retrieve')
    arguments.add_argument('--local-build-num', type=int, dest='local_num',
                           default=0, help='Build number to use for local files')
    arguments.add_argument('--pattern', type=str,
                           help='Artifact pattern (or exact name) to get',
                           nargs='*', default=['*.csv'])
    arguments.add_argument('--log', type=str, dest='loglevel',
                           help='Log level', default="info")
    arguments.add_argument('infile', nargs='*',
                           help="a local iperf CSV file",
                           type=argparse.FileType('r'), default=[])
    args = arguments.parse_args()

    numeric_level = getattr(logging, args.loglevel.upper(), None)
    if not isinstance(numeric_level, int):
        raise ValueError('Invalid log level: %s' % loglevel)
    logging.basicConfig(level=numeric_level)

    plotly.tools.set_credentials_file(username = args.plot_username,
                                      api_key = args.plot_key)
    
    fs = []
    if args.num_builds == 0:
        logging.info('Skipping CircleCI builds')
    else:
        for build_num in get_latest_builds(args):
            artifacts = get_artifact_list(args, build_num)
            for name, url in artifacts.items():
                if any(fnmatch(name, pattern) for pattern in args.pattern):
                    logging.info('Retrieving {build_num}:{name}...'.format(
                        name=name, build_num=build_num))
                    stats = BuildStats(num=int(build_num), file=parse_log(urlopen(url)))
                    fs.append()
                else:
                    logging.debug('Skipping {name}'.format(name=name))

    if len(args.infile) > 0:
        try:
            first_local_num = fs[-1].num
        except IndexError:
            n = int(args.local_num)
            logging.info('Local files from build_num={}'.format(n))
            first_local_num = n
        last_local_build = first_local_num + len(args.infile)
        local_range = range(first_local_num, last_local_build)
        fs += [BuildStats(x[0], x[1]) for x in zip(local_range, args.infile)]
    
    try:
        throughputs = [(f.num, average_throughtput_mbps(parse_log(f.file))) for f in fs]
        logging.debug("Obtained {} throughputs values".format(len(throughputs)))
    except TypeError, e:
        logging.critical("could not parse file {name}: {e}".format(name=f, e=e))
        sys.exit(1)
        
    if len(throughputs) == 0:
        logging.warning("no performance data obtained")
        sys.exit(1)
    else:
        build_nums, throughputs = zip(*throughputs)
        trace0 = Scatter(x = build_nums,
                        y = throughputs,
                        mode = 'lines+markers',
                        name = args.plot_desc,
                        line = Line(
                            shape='linear'
                        ))
        data = Data([trace0])
        layout = Layout(
            xaxis = XAxis(
                title = 'Build number',
                autotick = False,
            ),
            yaxis = YAxis(
                title = 'Throughput (MBps)',
            ),
            legend = Legend(
                y = 0.5,
                traceorder = 'reversed',
                font = Font(
                    size=16
                ),
                yref='paper'
            )
        )
        fig = Figure(data=data, layout=layout)

        logging.info("Plotting to plotly...")
        unique_url = py.plot(fig, filename = args.plot_name)
        logging.info("Plot can be found at {}".format(unique_url))
        if args.plot_save_to:
            logging.info("Saving PNG to {}".format(args.plot_save_to))
            args.plot_save_to.write(requests.get("{}.png".format(unique_url)).content)


