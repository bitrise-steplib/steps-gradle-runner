# Gradle Runner

[![Step changelog](https://shields.io/github/v/release/bitrise-io/steps-gradle-runner?include_prereleases&label=changelog&color=blueviolet)](https://github.com/bitrise-io/steps-gradle-runner/releases)

Runs a specified Gradle task.

<details>
<summary>Description</summary>

The Step runs the specified Gradle task and copies the generated APK and AAB files into
the Bitrise Deploy Directory (`$BITRISE_DEPLOY_DIR`). It is capable of doing everything that you can do with Gradle on your own machine.

### Configuring the Step

To use this Step, you need at least two things:

* [Gradle Wrapper](https://docs.gradle.org/current/userguide/gradle_wrapper.html).
* A Gradle task that is correctly configured in your Gradle project.

The Step can run the specified task with the Gradle Wrapper. For the basic configuration:

1. Open the **Config** input group.
1. In the **Gradle task to run** input, add the task you want to run. Without an existing, valid task, the Step will fail.
1. Set the `gradlew` file path: this is the path where the Gradle Wrapper lives in your project. The path should be relative to the project's root.
1. Optionally, you can set a `build.gradle` file for the Step in the **Optional path to the gradle build file to use** input.

To configure exporting an APK or AAB file generated by the Step:

1. Open the **Export config** input group.
1. Filter the files you want the Step to export. You can filter:
   * APK and AAB files.
   * Test APK files.
   * Mapping files.
   Both exclude and include filters can be used. Each filter option can take multiple patterns, with each pattern on its own line in the input field.

### Troubleshooting

If the Step keeps failing because it can't download the dependencies, check the `repositories` section in your `build.gradle` file.
It's possible that one or more of the services listed there are down so we Bitrise can't connect to them to download the dependencies you need.

If you use a `build.gradle` file and get the error `Issue with input: GradleFile does not exist`, check the **Optional path to the gradle build file to use** input.
Remember, the path must be relative to the root of the repository.

### Useful links

* [Gradle Wrapper](https://docs.gradle.org/current/userguide/gradle_wrapper.html)
* [Caching Gradle](https://devcenter.bitrise.io/builds/caching/caching-gradle/)

### Related Steps

* [Generate Gradle Wrapper](https://www.bitrise.io/integrations/steps/generate-gradle-wrapper)
* [Gradle Unit Test](https://www.bitrise.io/integrations/steps/gradle-unit-test)
* [Android Build](https://www.bitrise.io/integrations/steps/android-build)
</details>

## 🧩 Get started

Add this step directly to your workflow in the [Bitrise Workflow Editor](https://devcenter.bitrise.io/steps-and-workflows/steps-and-workflows-index/).

You can also run this step directly with [Bitrise CLI](https://github.com/bitrise-io/bitrise).

### Examples

This configuration builds all variant's `aab`:

```yaml
- gradle-runner@2:
    inputs:
    - gradlew_path: "./gradlew"
    - gradle_task: bundleRelease
```
You can also set up file path filters to avoid exporting unwanted archives or mapping files:

```yaml
- gradle-runner@2:
    inputs:
    - gradlew_path: "./gradlew"
    - gradle_task: bundleRelease
    - app_file_include_filter: "*release.aab"
    - app_file_exclude_filter: "*/temporary/*"
    - test_apk_file_include_filter: "*Test*.apk"
    - test_apk_file_exclude_filter: "*/immediate/*"
    - mapping_file_include_filter: "*/mapping.txt"
    - mapping_file_exclude_filter: "*/tmp/*"
```


## ⚙️ Configuration

<details>
<summary>Inputs</summary>

| Key | Description | Flags | Default |
| --- | --- | --- | --- |
| `gradle_file` | Optional path to the Gradle build file to use. It should be relative to the root of the project.  |  | `$GRADLE_BUILD_FILE_PATH` |
| `gradle_task` | Gradle task to run. You can call `gradle tasks` or `gradle tasks --all` in your Gradle project directory to get the list of available tasks.  | required | `assemble` |
| `gradlew_path` | Using a Gradle Wrapper (gradlew) is required, as the wrapper ensures that the right Gradle version is installed and used for the build. You can find more information about the Gradle Wrapper (gradlew), and about how you can generate one in the official guide at: [https://docs.gradle.org/current/userguide/gradle_wrapper.html](https://docs.gradle.org/current/userguide/gradle_wrapper.html). The path should be relative to the repository root. For example, `./gradlew`, or if it is in a sub directory, `./sub/dir/gradlew`.  | required | `$GRADLEW_PATH` |
| `app_file_include_filter` | The Step will copy the generated APK and AAB files that match this filter into the Bitrise deploy directory. Seperate patterns with a newline. Example: Copy every APK and AAB file: ``` *.apk *.aab ``` Copy every APK file with a filename that contains `release`, like (`./app/build/outputs/apk/app-release-unsigned.apk`): ``` *release*.apk ```  |  | `*.apk *.aab ` |
| `app_file_exclude_filter` | One filter per line. The Step will NOT copy the generated APK and AAB files that match these filters into the Bitrise deploy directory. You can use this filter to avoid moving unaligned and/or unsigned APK and AAB files. If you specify an empty filter, every APK and AAB file (selected by `APK and AAB file include filter`) will be copied. Seperate patterns with a newline. Examples: Do not copy APK files with a filename that contains `unaligned`: ``` *unaligned*.apk ``` Do not copy APK files with a filename that contains `unaligned` and/or `Test`: ``` *unaligned*.apk *Test*.apk ```  |  | `*unaligned.apk *Test*.apk */intermediates/* ` |
| `test_apk_file_include_filter` | The Step will copy the generated apk files that match this filter into the Bitrise deploy directory.  Example: Copy every APK if its filename contains Test, like (./app/build/outputs/apk/app-debug-androidTest-unaligned.apk):  ``` *Test*.apk ```  |  | `*Test*.apk` |
| `test_apk_file_exclude_filter` | One filter per line. The Step will NOT copy the generated apk files that match this filters into the Bitrise deploy directory. You can use this filter to avoid moving unalinged and/or unsigned apk files. If you specify an empty filter, every APK file (selected by `apk_file_include_filter`) will be copied. Example: Do not copy the test APK file if its filename contains `unaligned`: ``` *unaligned*.apk ```  |  |  |
| `mapping_file_include_filter` | The Step will copy the generated mapping files that match this filter into the Bitrise deploy directory. If you specify an empty filter, no mapping files will be copied. Example:  Copy every mapping.txt file: ``` *mapping.txt ```  |  | `*/mapping.txt` |
| `mapping_file_exclude_filter` | The Step will **not** copy the generated mapping files that match this filter into the Bitrise deploy directory. You can use this input to avoid moving a beta mapping file, for example. If you specify an empty filter, every mapping file (selected by `mapping_file_include_filter`) will be copied. Example:  Do not copy any mapping.txt file that is in a `beta` directoy: ``` */beta/mapping.txt ```  |  | `*/tmp/*` |
| `cache_level` | `all` - will cache build-cache and dependencies `only_deps` - will cache dependencies only `none` - won't cache any of the above | required | `only_deps` |
| `gradle_options` | Options added to the end of the Gradle call. You can use multiple options, separated by a space character. Example: `--stacktrace --debug` If `--debug` or `-d` options are set then only the last 20 lines of the raw gradle output will be visible in the build log. The full raw output will be exported to the `$BITRISE_GRADLE_RAW_RESULT_TEXT_PATH` variable and will be added as an artifact. |  | `--stacktrace --no-daemon` |
| `collect_metrics` | Enable Gradle metrics collection and send data to Bitrise.  |  | `no` |
</details>

<details>
<summary>Outputs</summary>

| Environment Variable | Description |
| --- | --- |
| `BITRISE_APK_PATH` | This output will include the path of the generated APK file, after filtering based on the filter inputs. If the build generates more than one APK file which fulfills the filter inputs this output will contain the last one's path. |
| `BITRISE_AAB_PATH` | This output will include the path of the generated AAB file, after filtering based on the filter inputs. If the build generates more than one AAB file which fulfills the filter inputs this output will contain the last one's path. |
| `BITRISE_TEST_APK_PATH` | This output will include the path of the generated test APK file, after filtering based on the filter inputs. If the build generates more than one APK file which fulfills the filter inputs this output will contain the last one's path. |
| `BITRISE_APK_PATH_LIST` | This output will include the paths of the generated APK files, after filtering based on the filter inputs. The paths are separated with `\|` character, eg: `app-armeabi-v7a-debug.apk\|app-mips-debug.apk\|app-x86-debug.apk` |
| `BITRISE_AAB_PATH_LIST` | This output will include the paths of the generated AAB files, after filtering based on the filter inputs. The paths are separated with `\|` character, eg: `app.aab\|app2.aab` |
| `BITRISE_MAPPING_PATH` | This output will include the path of the generated mapping.txt. If more than one mapping.txt exist in project this output will contain the last one's path. |
</details>

## 🙋 Contributing

We welcome [pull requests](https://github.com/bitrise-io/steps-gradle-runner/pulls) and [issues](https://github.com/bitrise-io/steps-gradle-runner/issues) against this repository.

For pull requests, work on your changes in a forked repository and use the Bitrise CLI to [run step tests locally](https://devcenter.bitrise.io/bitrise-cli/run-your-first-build/).

Learn more about developing steps:

- [Create your own step](https://devcenter.bitrise.io/contributors/create-your-own-step/)
- [Testing your Step](https://devcenter.bitrise.io/contributors/testing-and-versioning-your-steps/)
