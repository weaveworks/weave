#!/bin/sh -ex

cd $(dirname $0)/

## Use GCS until we set up Route53/CloudFront/S3

BRANCH=${TRAVIS_BRANCH:-`git rev-parse --abbrev-ref @`}
COMMIT=${TRAVIS_COMMIT:-`git rev-parse @`}

export BRANCH COMMIT

bundle install --path=.bundle
bundle exec jekyll build --verbose
gsutil -m cp -z html,css -a public-read -R _site gs://docs.weave.works/weave/${BRANCH}/${COMMIT}
