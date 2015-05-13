#!/bin/sh -ex

cd $(dirname $0)/

## Use GCS until we set up Route53/CloudFront/S3

BRANCH=${TRAVIS_BRANCH:-`git rev-parse --abbrev-ref @`}
COMMIT=${TRAVIS_COMMIT:-`git rev-parse @`}
OUTPUT="weave/${BRANCH}/${COMMIT}"

if [ -n "${TRAVIS_TAG}" ]; then
  ## Travis will run separate build for commit and tag pushes,
  OUTPUT="weave/${TRAVIS_TAG}"
fi

if [ "${BRANCH}" = "latest_release_doc_updates" ]; then
  OUTPUT="weave/latest_release"
fi

export BRANCH COMMIT OUTPUT

bundle install --path=.bundle
bundle exec jekyll build --verbose

gsutil -m rsync -r -d _site "gs://docs.weave.works/${OUTPUT}"

echo "Published at http://docs.weave.works/${OUTPUT}"

if [ -z "${TRAVIS_TAG}" -a ! "${BRANCH}" = "latest_release_doc_updates" ]; then
  echo "<meta http-equiv=\"refresh\" content=\"0; url=http://docs.weave.works/${OUTPUT}\" />" \
    | gsutil \
      -h "Content-Type:text/html" \
      -h "Cache-Control:private, max-age=0, no-transform" \
      cp -a "public-read" - "gs://docs.weave.works/weave/${BRANCH}/index.html"
fi
