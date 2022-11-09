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
docker tag detour2:latest registry.cn-hongkong.aliyuncs.com/hjcrocks/detour2:latest
docker push registry.cn-hongkong.aliyuncs.com/hjcrocks/detour2:latest

echo "git tag and push"
sed -i -r 's/VERSION =.*/VERSION = "'$VERSION'"/g' version.go
git commit -a -m "chore: pump to version $VERSION"
git tag v"$1" -f
git push --tag -f
