#!/bin/bash
# usage: fakeparent.sh NAME cmd...  — run cmd as the child of a shell whose argv[0] is NAME.
# The trailing `exit $?` stops sh from exec-optimising itself away.
name=$1; shift
exec -a "$name" /bin/sh -c '"$0" "$@"; exit $?' "$@"
