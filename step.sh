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

if [ -z "$gradlew_path" ] ; then
	printf "\e[31m [!] No gradlew_path specified\e[0m\n"
	echo
	echo "Using a Gradle Wrapper (gradlew) is required, as the wrapper is what makes sure"
	echo "that the right Gradle version is installed and used for the build."
	echo
	echo "You can find more information about the Gradle Wrapper (gradlew),"
	echo "and about how you can generate one (if you would not have one already)"
	echo "in the official guide at: https://docs.gradle.org/current/userguide/gradle_wrapper.html"
	exit 1
fi

if [ -z "${apk_file_include_filter}" ]; then
	apk_file_include_filter="*.apk"
fi

if [ -z "${apk_file_exclude_filter}" ]; then
	apk_file_exclude_filter=""
fi


if [ ! -z "${workdir}" ] ; then
	echo
	echo "=> Switching to specified workdir"
	echo '$' cd "${workdir}"
	cd "${workdir}"
fi

gradle_tool=""
if [ ! -f "$gradlew_path" ] ; then
	echo " [!] gradlew_path specified, but no file exists at the specified path: ${gradlew_path}"
	exit 1
fi
if [ ! -x "$gradlew_path" ] ; then
	echo " (i) Missing executable permission on gradlew file, adding it now. (path: $gradlew_path)"

	chmod +x "$gradlew_path"
fi
gradle_tool="$gradlew_path"

echo
echo "=== CONFIGURATION ==="
echo " * Using gradle tool: ${gradle_tool}"
echo " * Gradle build file: ${gradle_file}"
echo " * Gradle task: ${gradle_task}"
echo " * Gradle options: ${gradle_options}"
echo " * Workdir: ${workdir}"

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
