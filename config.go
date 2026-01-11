package main

const (
	defaultDirSrc = "E:\\DCIM\\100MSDCF"
	defaultDirDst = "D:\\raw"

	defaultFolderCreationThresholdOneDay          = 600
	defaultFolderCreationThresholdConsecutiveDays = 300

	dateFormat = "200601-02"
)

var (
	// EditFileExtensions contains file extensions for edit sidecar files (e.g., Lightroom's XMP)
	EditFileExtensions = []string{"xmp"}

	// ExtensionsToCopy contains raw file extensions to copy from SD card
	ExtensionsToCopy = []string{"arw", "raw"}
)
