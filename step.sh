#!/bin/bash

set -e

if [ -z "${gradle_task}" ]; then
	printf "\e[31mNo gradle task found\e[0m\n"
	exit 1
fi

echo "$" gradle ${gradle_task}
gradle ${gradle_task}