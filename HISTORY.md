# History of the ReWeave project

## How it started

In June 2022, Weave Net had not been updated for a year. Problems were starting to appear in the field. In particular, the last published images on the Docker Hub ([weaveworks/weave-kube](https://hub.docker.com/r/weaveworks/weave-kube) and [weaveworks/weave-npc](https://hub.docker.com/r/weaveworks/weave-npc), v2.8.1) had issues supporting multiple processor architectures, and security scanners showed multiple vulnerabilities. 

A call went out from Weaveworks to get the community more involved in maintaining it. After some discussion on GitHub issues and e-mail, and even a few online meetings, things were not moving forward. 

## ReWeave begins

Finally, in March 2023, this fork was created, with the following goals in mind:

* Update dependencies, especially ones with security vulnerabilities
* Make minimal code changes _only_ when required by updating dependencies
* Create true multi-arch images using modern tools
* Create a new build process to automate all this
* Do all this with _minimal_ changes to the existing codebase. Keep all new things in the `reweave` folder.

These goals were achieved. Details can be found in the [reweave](reweave/README.md) directory. A [pull request](https://github.com/weaveworks/weave/pull/3996) was submitted on the weaveworks repo, with the aim of getting a new official release out.

## Weaveworks ends

On February 5th, 2024, Weaveworks CEO Alexis Richardson announced via [LinkedIn](https://www.linkedin.com/posts/richardsonalexis_hi-everyone-i-am-very-sad-to-announce-activity-7160295096825860096-ZS67/) and [Twitter](https://twitter.com/monadic/status/1754530336120140116) that Weaveworks is winding down. 

So, a decision was taken to maintain this fork independently.

## We're forked

Two major changes were introduced at this point:

* The module name was changed to `github.com/rajch/weave` (previously `github.com/weaveworks/weave`)
* The default registry account for publishing images was changed to `docker.io/rajchaudhuri` (previously `docker.io/weaveworks`)

In addition, the old repo structure and codebase is not longer sacrosanct. Things can be moved around, new code can be added outside the `reweave` directory, old code can be modified or deleted as necessary.

The version numbers will continue from where Weaveworks left off. 

## New Goals

The old goals, listed above (except the last one), remain the priority. In addition, this project aims to:

* Remove dependencies on Weaveworks infrastructure, starting with telemetry (what weaveworks called checkpoint)
* Publish new releases regularly, duly security scanned
* Provide supporting infrastructure, such as weave's famous one-line installation, where possible
