# Use of vendored code in Weave Net

Weave Net is licensed under the [Apache 2.0 license](LICENSE).

Some vendored code is under different licenses though, all of them ship the
entire license text they are under.

- https://github.com/weaveworks/go-checkpoint
  can be found in the ./vendor/ directory, is under MPL-2.0.

- Pulled in by dependencies are  
  https://github.com/hashicorp/golang-lru (MPL-2.0)  
  https://github.com/hashicorp/go-cleanhttp (MPL-2.0)  
  https://github.com/opencontainers/go-digest (docs are under CC by-sa 4.0)

[One file used in tests](COPYING.LGPL-3) is under LGPL-3, that's why we ship
the license text in this repository.
