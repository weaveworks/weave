#!/usr/bin/python

# List all available versions of Weave Net's dependencies:
# - Go
# - Docker
# - Kubernetes
#
# Depending on the parameters passed, it can gather the equivalent of the below bash one-liners:
#   git ls-remote --tags https://github.com/golang/go | grep -oP '(?<=refs/tags/go)[\.\d]+$'      | sort --version-sort
#   git ls-remote --tags https://github.com/golang/go | grep -oP '(?<=refs/tags/go)[\.\d]+rc\d+$' | sort --version-sort | tail -n 1
#   git ls-remote --tags https://github.com/docker/docker | grep -oP '(?<=refs/tags/v)\d+\.\d+\.\d+$'        | sort --version-sort
#   git ls-remote --tags https://github.com/docker/docker | grep -oP '(?<=refs/tags/v)\d+\.\d+\.\d+\-rc\d*$' | sort --version-sort | tail -n 1
#   git ls-remote --tags https://github.com/kubernetes/kubernetes | grep -oP '(?<=refs/tags/v)\d+\.\d+\.\d+$'            | sort --version-sort
#   git ls-remote --tags https://github.com/kubernetes/kubernetes | grep -oP '(?<=refs/tags/v)\d+\.\d+\.\d+\-beta\.\d+$' | sort --version-sort | tail -n 1
#
# Dependencies:
# - python
# - git
# - grep (GNU)
# - sort (GNU)

from os import linesep
from sys import argv, exit, stdout, stderr
from getopt import getopt, GetoptError
from subprocess import Popen, PIPE, STDOUT
import shlex

ERROR_RUNTIME = 1
ERROR_ILLEGAL_ARGS = 2

DEPS={
    'go': {
        'url':  'https://github.com/golang/go',
        'grep': '(?<=refs/tags/go)[\.\d]+(rc\d)*$',
        'rc':   'rc'
    },
    'docker': {
        'url':  'https://github.com/docker/docker',
        'grep': '(?<=refs/tags/v)\d+\.\d+\.\d+(\-rc\d)*$',
        'rc':   'rc',
        'min':  '1.6.0'  # Weave Net only works with Docker from 1.6.0 onwards, so we ignore all previous versions.
    },
    'kubernetes': {
        'url':  'https://github.com/kubernetes/kubernetes',
        'grep': '(?<=refs/tags/v)\d+\.\d+\.\d+(\-beta\.\d)*$',
        'rc':   'beta'
    }
}

def validate(process, error_msg):
    process.wait()
    if process.returncode != 0:
        out, err = process.communicate()
        raise RuntimeError('%s. Output: %s. Error: %s' % (error_msg, out, err))
    return process

def sanitize(out):
    return out.decode('ascii').strip().split(linesep)

def partition(predicate, iterable):
    ok = []
    ko = []
    for elem in iterable:
        if predicate(elem):
            ok.append(elem)
        else:
            ko.append(elem)
    return ok, ko

def get_versions(git_repo_url, grep_pattern, rc_tag, include_rc=False):
    git  = validate(
        Popen(shlex.split('git ls-remote --tags %s' % git_repo_url), stdout=PIPE),
        'Failed to retrieve git tags from %s' % git_repo_url
    )
    grep = validate(
        Popen(shlex.split('grep -oP "%s"' % grep_pattern), stdin=git.stdout, stdout=PIPE),
        'Failed to grep versions'
    )
    sort = validate(
        Popen(shlex.split('sort --version-sort'), stdin=grep.stdout, stdout=PIPE),
        'Failed to sort version in increasing order'
    )
    out, _ = sort.communicate()
    versions = sanitize(out)
    rc, prod = partition(lambda v: rc_tag in v, versions)
    if include_rc:
        latest_rc_version = next(reversed(rc))
        return prod + [latest_rc_version]
    else:
        return prod

def usage(error_message=None):
    if error_message:
        stderr.write('ERROR: ' + error_message + linesep)
    stdout.write(linesep.join([
        'Usage:',
        '    list_versions.py [OPTION]... [DEPENDENCY]',
        'Examples:',
        '    list_versions.py docker',
        '    list_versions.py --include-rc docker',
        'Options:',
        '--include-rc    Include latest release candidate version.',
        '-h/--help       Prints this!',
        ''
    ]))

def validate_input(argv):
    try:
        include_rc=False
        opts, args = getopt(argv, 'h', ['help', 'include-rc'])
        for opt, value in opts:
            if opt in ('-h', '--help'):
                usage()
                exit()
            if opt == '--include-rc':
                include_rc=True
        if len(args) != 1:
            raise ValueError('Please provide a dependency to get versions of. Expected 1 argument but got %s: %s.' % (len(args), args))
        dependency=args[0].lower()
        if dependency not in DEPS.keys():
            raise ValueError('Please provide a valid dependency. Supported one dependency among {%s} but got: %s.' % (', '.join(DEPS.keys()), dependency))
        return dependency, include_rc
    except GetoptError as e:
        usage(str(e))
        exit(ERROR_ILLEGAL_ARGS)
    except ValueError as e:
        usage(str(e))
        exit(ERROR_ILLEGAL_ARGS)

def main(argv):
    try:
        dependency, include_rc=validate_input(argv)
        versions = get_versions(
            DEPS[dependency]['url'], 
            DEPS[dependency]['grep'], 
            DEPS[dependency]['rc'], 
            include_rc
        )
        if 'min' in DEPS[dependency]:
            index = versions.index(DEPS[dependency]['min'])
            versions = versions[index:]
        print(linesep.join(versions))
    except Exception as e:
        print(str(e))
        exit(ERROR_RUNTIME)

if __name__ == '__main__':
    main(argv[1:])
