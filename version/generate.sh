#!/bin/sh

version=`cat version.txt`
cat > version.go <<EOF
package version

// This file is generated from version.txt. Do not edit directly.
const APP_VERSION="$version"
EOF
