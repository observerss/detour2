#!/bin/bash
set -ex
if [ "$1" == "" ]; then
    echo "usage: ./buish.sh VERSION"
    exit
fi
VERSION="$1"
docker build -t detour2 .
docker save -o dist/detour2.tar detour2
rm -f dist/detour2.tar.gz; gzip dist/detour2.tar
docker tag detour2:latest registry.cn-hongkong.aliyuncs.com/hjcrocks/detour2:$VERSION
docker push registry.cn-hongkong.aliyuncs.com/hjcrocks/detour2:$VERSION