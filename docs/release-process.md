# Release Process
## Prerequisites

* Up-to-date versions of `git`, `go` etc
* Install GitHub release tool `go get github.com/weaveworks/github-release`
* Create a [github token for
  github-release](https://help.github.com/articles/creating-an-access-token-for-command-line-use/);
set and export `$GITHUB_TOKEN` with this value
* Update all dependencies with `make update`

## Build Phase
### Update CHANGELOG.md

* Checkout the branch from which you wish to release
* Choose an appropriate version tag, henceforth referred to as `$TAG`.
  Mainline releases use a version number (e.g. `TAG=v2.0.0`), whereas
  pre-releases get a descriptive name (e.g. `TAG=feature-preview-20150902`)
* Add a changelog entry for the new tag at the top of `CHANGELOG.md`.
  The first line must be a markdown header of the form `## Release
  $TAG`

Commit the changelog update:

    git commit -m "Add release $TAG" CHANGELOG.md

### Create Version Tag

Next you must tag the changelog commit with `$TAG`

    git tag -a -m "Release $TAG" $TAG

### Execute Build

You are now ready to perform the build. If you have skipped the
previous steps (e.g. because you're doing a rebuild), you must ensure
that `HEAD` points to the tagged commit. You may then execute

    bin/release build

This has the following effects:

* `git tag --points-at HEAD` is used to determine `$TAG` (hence the
  `HEAD` requirement)
* Your *local* repository is cloned into `releases/$TAG`
* `CHANGELOG.md` is checked to ensure it has an entry for `$TAG`
* Distributables injected with `$TAG` are built
* Tests are executed

## Draft Phase
### Push Version Tag Upstream

First you must push your version tag upstream, so that an associated
GitHub release may be created:

    git push git@github.com:weaveworks/weave $TAG

N.B. if you're testing the release process, push to your fork
instead!

### Create Draft Release

You're now ready to draft your release notes:

    bin/release draft [--pre-release]

This has the following effects:

* A [release](https://help.github.com/articles/about-releases) is
  created in GitHub for `$TAG`. This release is in the draft state, so
  it is only visible to contributors
* The `weave` script is uploaded as an attachment to the release
* If `--pre-release` is specified, the release will have the
  pre-release attribute set (this affects the way GitHub displays the
  release and modifies the behaviour of the publish phase)

Navigate to https://github.com/weaveworks/weave/releases, 'Edit' the
draft and input the release notes. When you are done make sure you
'Save draft' (and not 'Publish release'!).

Once the release notes have passed review, proceed to the publish
phase.

## Publish Phase
### Move/Force Push `latest_release` Tag

This step must only be performed for mainline (non pre-release)
releases:

    git tag -af -m "Release $TAG" latest_release $TAG
    git push -f git@github.com:weaveworks/weave latest_release

The `latest_release` tag *must* point at `$TAG`, *not* at `HEAD` -
the build script will complain otherwise.

N.B. if you're testing the release process, push to your fork
instead!

### Publish Release & Distributable Artefacts

You can now publish the release and upload the remaining
distributables to DockerHub:

    bin/release publish

This has the following effects:

* Docker images are tagged `$TAG` and pushed to DockerHub
* GitHub release moves from draft to published state

Furthermore, if this is a mainline release (detected automatically
from the GitHub release, you do not need to specify the flag again to
the publish step)

* Images tagged `latest` are updated on DockerHub
* Release named `latest_release` is updated on GitHub


## Troubleshooting

There's a few things that can go wrong.

 * If the build is wonky, e.g., the tests don't pass, you can delete
   the directory in `./releases/`, fix whatever it is, move the
   version tag (which should still be only local) and have another go.
 * If the DockerHub pushes fail (which sadly seems to happen a lot),
   you can just run `./bin/release publish` again.
 * If you need to overwrite a release you can do so by manually
   deleting the GitHub version release and re-running the process.
   Please note that the DockerHub `latest` images, GitHub
   `latest_release` and download links may be in an inconsistent state
   until the overwrite is completed.
