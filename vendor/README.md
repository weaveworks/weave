
# Managing Vendored Dependencies with `git submodule`

These operations result in staged but uncommitted changes to your
branch; you will need to commit them as normal. Execute them in the
root of your checkout.

## Adding a New Dependency

    ~/weave$ git submodule add https://example.com/organisation/module vendor/example.com/organisation/module

## Update All Dependencies to Upstream `master`

    ~/weave$ git submodule foreach git pull origin master

## Force a Specific Dependency to a Particular Commit

	~/weave$ (cd vendor/example.com/organisation/module && git checkout <commit-ish>)
	~/weave$ git add vendor/example.com/organisation/module

## Remove a Dependency

	~/weave$ git submodule deinit vendor/example.com/organisation/module
    ~/weave$ git rm vendor/example.com/organisation/module
	~/weave$ rm -rf .git/modules/vendor/example.con/organisation/module

