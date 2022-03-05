#!/usr/bin/env sh
#
# Install contents from an apk archive

set -e

[ -f .pre-install ] && (./.pre-install && rm .pre-install) || true

find . -not -path ./.tarball -not -path ./.PKGINFO -not -path "./.SIGN.RSA.*" -not -path ./.post-install | cpio -pdmuv /

[ -f .post-install ] && (./.post-install && rm .post-install) || true
