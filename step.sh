#!/bin/bash

set -e

if [ -z "${gradle_file}" ]; then
	printf "\e[31mNo gradle_file specified\e[0m\n"
	exit 1
fi

if [ -z "${gradle_task}" ]; then
	printf "\e[31mNo gradle_task specified\e[0m\n"
	exit 1
fi

echo "$" gradle --build-file "${gradle_file}" ${gradle_task}
gradle --build-file "${gradle_file}" ${gradle_task}