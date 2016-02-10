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

if [ ! -z "${workdir}" ] ; then
	echo
	echo "=> Switching to specified workdir"
	echo '$' cd "${workdir}"
	cd "${workdir}"
fi

if [ -z "${apk_file_include_filter}" ]; then
	apk_file_include_filter="*.apk"
fi

if [ -z "${apk_file_exclude_filter}" ]; then
	apk_file_exclude_filter=""
fi

gradle_tool=gradle
if [ ! -z "$gradlew_path" ] ; then
	gradle_tool="$gradlew_path"

	if [ ! -x "$gradlew_path" ] ; then
		echo " (i) Missing executable permission on gradlew file, adding it now. Path: $gradlew_path"

		chmod +x "$gradlew_path"
	fi
fi

echo
echo "=== CONFIGURATION ==="
echo " * Using gradle tool: ${gradle_tool}"
echo " * Gradle build file: ${gradle_file}"
echo " * Gradle task: ${gradle_task}"
echo " * Gradle options: ${gradle_options}"

echo
echo "=> Running gradle task ..."
set -x
${gradle_tool} --build-file "${gradle_file}" ${gradle_task} ${gradle_options}
set +x

echo
echo "=> Moving APK files with filter: include-> '${apk_file_include_filter}', exclude-> '${apk_file_exclude_filter}'"
last_moved_apk_pth=""
find_apks_output="$(find . -name "${apk_file_include_filter}" ! -name "${apk_file_exclude_filter}")"
if [[ "${find_apks_output}" != "" ]] ; then
	while IFS= read -r apk
	do
		deploy_path="${BITRISE_DEPLOY_DIR}/$(basename "$apk")"

		printf "🚀  \e[32mCopy ${apk} to ${deploy_path}\e[0m\n"
		cp "${apk}" "${deploy_path}"
		last_moved_apk_pth="${deploy_path}"
	done <<< "${find_apks_output}"
fi

if [[ "${last_moved_apk_pth}" != "" ]] ; then
	echo 'Exporting output: $BITRISE_APK_PATH =>' "${last_moved_apk_pth}"
	envman add --key "BITRISE_APK_PATH" --value "${last_moved_apk_pth}"
else
	echo " (!) No APK matched the filters."
fi

last_moved_mapping_pth=""
find_mappings_output="$(find . -path "${mapping_file_include_filter}" ! -path "${mapping_file_exclude_filter}")"
if [[ "${find_mappings_output}" != "" ]] ; then
	echo
	echo "=> Moving mapping with filter: include-> '${mapping_file_include_filter}', exclude-> '${mapping_file_exclude_filter}'"

	while IFS= read -r mapping
	do
		deploy_path="${BITRISE_DEPLOY_DIR}/$(basename "$mapping")"

		printf "🚀  \e[32mCopy ${mapping} to ${deploy_path}\e[0m\n"
		cp "${mapping}" "${deploy_path}"
		last_moved_mapping_pth="${deploy_path}"
	done <<< "${find_mappings_output}"
fi

if [[ "${last_moved_mapping_pth}" != "" ]] ; then
	echo 'Exporting output: $BITRISE_MAPPING_PATH =>' "${last_moved_mapping_pth}"
	envman add --key "BITRISE_MAPPING_PATH" --value "${last_moved_mapping_pth}"
fi

echo
echo "=> DONE"
