# Clean SD Card

A simple Go utility designed to offload RAW image files from an SD card to a local drive and clean up the source directory.

## Description

This tool scans a source directory for files with `.arw` or `.raw` extensions. It copies them to a destination directory and then removes **all** files from the source directory to free up space.

## Features

- **Copy:** Safely copies `.arw` and `.raw` files to the destination.
- **Clean:** Removes all files from the source directory after processing.
- **Zombie Edit File Cleanup:** Automatically removes orphaned `.xmp` edit files (Lightroom sidecar files) that no longer have a corresponding RAW file.
- **Dry Run:** Simulate the process to see what would happen without making actual changes.
- **Overwrite Control:** Option to overwrite existing files in the destination.

## Usage

### Prerequisites

- Go installed on your machine.

### Running the tool

You can run the tool directly using `go run`:

```bash
go run main.go [flags]
```

### Flags

- `-src`: Source directory (default: `E:\DCIM\100MSDCF`).
- `-dst`: Destination directory (default: `D:\raw`).
- `-dry-run`: Simulate operations without modifying any files. Useful for verification.
- `-overwrite`: Overwrite existing files in the destination directory. Default behavior skips existing files.
- `-delete-zombie-edit-files`: Delete orphaned `.xmp` edit files that have no corresponding RAW file (default: `true`).

### Examples

**1. Dry Run (Safe Mode)**
Check what files would be copied and removed without actually doing it:
```bash
go run main.go -dry-run
```

**2. Standard Run**
Copy files and clean the SD card (skips existing files in destination):
```bash
go run main.go
```

**3. Overwrite Existing Files**
Copy files and overwrite duplicates in the destination:
```bash
go run main.go -overwrite
```

**4. Custom Source and Destination**
Specify custom directories:
```bash
go run main.go -src /path/to/sd/card -dst /path/to/backup
```

**5. Skip Zombie Edit File Cleanup**
Keep orphaned `.xmp` files in the destination:
```bash
go run main.go -delete-zombie-edit-files=false
```
