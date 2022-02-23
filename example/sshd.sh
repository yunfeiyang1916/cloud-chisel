#!/bin/sh
docker run -d -p 2202:22 --name sshd sickp/alpine-sshd
