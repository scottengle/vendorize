vendorize
=========

vendorize is a tool for vendorizing go imports, including transitive dependencies

What does it do?
================
vendorize crawls the dependency graph of a given package and copies external dependencies
to a specified import prefix. It handles transitive dependencies, and updates the import
statements of all packages to point to the right place.

How do I use it?
================

First, install vendorize using the standard `go get` command:

    $ go get github.com/scottengle/vendorize

Next, select a project whose dependencies you want to vendorize.
Select a package import path prefix where the dependencies will be copied.
These two paths make up the two mandatory positional arguments to vendorize.

Run the tool in "dry run" mode with the `-d` switch. This will give you a log of what *would*
happen, but does not actually make any changes to your package:

	$ vendorize -d -b blacklisted.directory.com/ github.com/project/repo github.com/project/repo/_vendor/src
	2014/08/14 11:09:09 Copying contents of "$GOPATH/src/github.com/andybons/hipchat" to "$GOPATH/src/github.com/project/repo/_vendor/src/github.com/andybons/hipchat"
	2014/08/14 11:09:09 Copying contents of "$GOPATH/src/github.com/cactus/go-statsd-client/statsd" to "$GOPATH/src/github.com/project/repo/_vendor/src/github.com/cactus/go-statsd-client/statsd"
	2014/08/14 11:09:09 Copying contents of "$GOPATH/src/github.com/codegangsta/inject" to "$GOPATH/src/github.com/project/repo/_vendor/src/github.com/codegangsta/inject"
	2014/08/14 10:43:09 Ignored (preexisting): "$GOPATH/src/github.com/project/repo/_vendor/src/github.com/go-martini/martini"
	2014/08/14 11:09:09 Copying contents of "$GOPATH/src/github.com/mipearson/rfw" to "$GOPATH/src/github.com/project/repo/_vendor/src/github.com/mipearson/rfw"

If you want to blacklist some paths from being vendorized, specify the prefix
with the `-b` flag. The flag can be given multiple times to ignore multiple
prefixes.

The vendorize tool won't overwrite packages that are already present in the vendorize
destination directory. To force it to do so, use the `-f` flag:

	$ vendorize -d -f -b blacklisted.directory.com/ github.com/project/repo github.com/project/repo/_vendor/src
	2014/08/14 11:09:09 Copying contents of "$GOPATH/src/github.com/andybons/hipchat" to "$GOPATH/src/github.com/project/repo/_vendor/src/github.com/andybons/hipchat"
	2014/08/14 11:09:09 Copying contents of "$GOPATH/src/github.com/cactus/go-statsd-client/statsd" to "$GOPATH/src/github.com/project/repo/_vendor/src/github.com/cactus/go-statsd-client/statsd"
	2014/08/14 11:09:09 Copying contents of "$GOPATH/src/github.com/codegangsta/inject" to "$GOPATH/src/github.com/project/repo/_vendor/src/github.com/codegangsta/inject"
	2014/08/14 10:43:09 Copying contents of "$GOPATH/src/github.com/go-martini/martini" to $GOPATH/src/github.com/project/repo/_vendor/src/github.com/go-martini/martini"
	2014/08/14 11:09:09 Copying contents of "$GOPATH/src/github.com/mipearson/rfw" to "$GOPATH/src/github.com/project/repo/_vendor/src/github.com/mipearson/rfw"

If you are satisfied with the output, simply remove the `-d` switch to have vendorize
copy the dependencies to the destination directory.

Currently, there are two best practice approaches to vendorizing 
a package. Peter Bourgon's excellent blog post on Go in production
covers both in detail (scroll down to Dependency Management):

http://peter.bourgon.org/go-in-production/

In support of these approaches, vendorize won't update import statements
without a flag to indicate that it should do so. Add `-u` to update
all the import statements for a vendorized package.

Updating an individual package
==============================

The easiest way to update a single vendor package is to simply
go get the updated source, delete the directory from the
destination directory and then re-run the vendorize command
(without the `-f` flag).

Differences from the upstream repo
==================================

This fork has a couple of major differences from the upstream repository
it was forked from.

*Flag changes:*

- `-n` is now `-d`
- `-ignore` is now `-b`
- `-f` and `-u` were added

*Behavioral Changes:*

- By default, vendorize won't overwrite packages that already exist in the destination directory
- By default, vendorize won't update import statements to vendorized packages

Future Plans
============

This is the list of planned updates to this repo (in no particular order):

- Ability to specify a whitelist of paths, targetting only those directory paths
- Tests
