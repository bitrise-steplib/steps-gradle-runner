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
    - app_file_exclude_filter: |-
        *unaligned.apk
        *Test*.apk
        */intermediates/*
        */temporary/*
    - test_apk_file_include_filter: "*Test*.apk"
    - mapping_file_include_filter: "*/mapping.txt"
    - mapping_file_exclude_filter: |-
        */tmp/*
        */intermediates/*
        *compose-mapping.txt
        */beta/*
```

A filter input replaces the Step's defaults instead of extending them, so the example above repeats the default patterns and adds the project-specific `*/temporary/*` and `*/beta/*` to them. Dropping `*/intermediates/*` would bring back the working copies Gradle keeps next to the published archives and mapping files, under the same file names.
