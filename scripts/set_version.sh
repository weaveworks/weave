#!/bin/bash

# adapted from: https://github.com/hesco/hesco-weave/blob/master/scripts/set_version.sh 
LEGACY_VERSION=`/bin/grep ^VERSION README.md | /bin/sed "s,^VERSION ,,"`

function v_version() {
  local VVERSION=$(/usr/bin/git describe --tags | /bin/sed "s,-,.," | /bin/sed "s,-.*$,,")
  if [ ! -f README.md ]
  then
    /bin/echo "Add a README.md file and try again."
    exit 1
  fi
  [ -f README.md ] && /bin/sed -i "s,^Version .*$,Version $VVERSION," README.md
  /bin/echo "$VVERSION"
}

function version() {
  local VVERSION=$(v_version)
  local VERSION=`/bin/echo $VVERSION | /bin/sed "s,^v,,"`
  if [ ! -f metadata.json ] && [ ! -f Modulefile ]
  then
    /bin/echo "Add a metadata.json file and try again."
    exit 1
  fi
  [ -f metadata.json ] && /bin/sed -i "s,^  \"version.*$,  \"version\": \"$VERSION\"\,," metadata.json 
  [ -f Modulefile ] && /bin/sed -i "s,^version .*$,version '$VERSION'," Modulefile 
  /bin/echo "$VERSION"
}

if [ `/usr/bin/git diff README.md metadata.json | wc -l` -gt 0 ]
then
  /bin/echo 'Commit or stash outstanding changes to metadata.json and README, or git checkout previous version, before release'
  exit 1
fi

VVERSION=$(v_version)

if [ "$LEGACY_VERSION" != "$VVERSION" ]
then
  /bin/echo "Use git tag to increment the minor version for new backward compatible functionality"
  /bin/echo "Use git tag to increment the major version for incompatible API changes"
  /bin/echo "Shall we git commit to the repository the incrementation of the version to $VVERSION [yes/NO]"
  read commit_incremented_version
  if [[ $commit_incremented_version == 'yes' ]]
  then
    /usr/bin/git commit README.md -m"make release used to update version to $VVERSION "
    VVERSION=$(v_version)
    /usr/bin/git commit --amend README.md -m"make release used to update version to $VVERSION "
    exit 0
  else
    exit 1
  fi
fi
exit 0

