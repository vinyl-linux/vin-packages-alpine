#!/usr/bin/env sh
#
# Install contents from an apk archive

[ -f .pre-install ] && ./.pre-install

rm .pre-install .PKGINFO *.pub

find . -name '*.csv' | cpio -pdm /
