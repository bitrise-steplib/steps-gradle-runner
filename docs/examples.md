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
