#!/bin/bash
docker build -t detour2 .
docker save -o dist/detour2.tar detour2
gzip dist/detour2.tar