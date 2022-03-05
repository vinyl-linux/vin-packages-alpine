#!/usr/bin/env sh
#
# Install contents from an apk archive

[ -f .pre-install ] && ./.pre-install && .pre-install

find . -not -path ./.tarball -not -path .PKGINFO -not -path ".SIGN.RSA.*" -not -path .post-install | cpio -pdm /

[ -f .post-install ] && ./.post-install && .post-install
