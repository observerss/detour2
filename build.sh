#!/bin/bash
if [ "$1" == "" ]; then
    echo "usage: ./buish.sh VERSION"
    exit
fi

set -ex
VERSION="$1"

echo "docker build"
docker build -t detour2 .
docker save -o dist/detour2.tar detour2
rm -f dist/detour2.tar.gz; gzip dist/detour2.tar

echo "docker push"
docker tag detour2:latest registry.cn-hongkong.aliyuncs.com/hjcrocks/detour2:$VERSION
docker push registry.cn-hongkong.aliyuncs.com/hjcrocks/detour2:$VERSION

echo "git tag and push"
git tag v"$1" -f
git push --tag -f
