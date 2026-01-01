# Clean SD Card

A simple Go utility designed to offload RAW image files from an SD card to a local drive and clean up the source directory.

## Description

This tool scans a specific source directory (hardcoded as `E:\DCIM\100MSDCF`) for files with `.arw` or `.raw` extensions. It copies them to a destination directory (hardcoded as `D:\raw`) and then removes **all** files from the source directory to free up space.

## Features

- **Copy:** Safely copies `.arw` and `.raw` files to the destination.
- **Clean:** Removes all files from the source directory after processing.
- **Dry Run:** Simulate the process to see what would happen without making actual changes.
- **Overwrite Control:** Option to overwrite existing files in the destination.

## Usage

### Prerequisites

- Go installed on your machine.
- Source path `E:\DCIM\100MSDCF` (or modify `main.go`).
- Destination path `D:\raw` (or modify `main.go`).

### Running the tool

You can run the tool directly using `go run`:

```bash
go run main.go [flags]
```

### Flags

- `-dry-run`: Simulate operations without modifying any files. Useful for verification.
- `-overwrite`: Overwrite existing files in the destination directory. Default behavior skips existing files.

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

## Configuration

Currently, the source and destination paths are hardcoded in the `const` block of `main.go`. To change these paths, edit the file and re-run the program.
