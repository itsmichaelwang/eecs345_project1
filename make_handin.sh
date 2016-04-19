#!/bin/sh
set -e
filename=kademlia-mzw462+gml654+aly155-`date "+%Y.%m.%d-%H.%M.%S"`.tar.gz
tar -zcvf ${filename} --exclude="*DS_Store" src
