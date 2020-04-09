package fatfs

// #include <string.h>
// #include <stdlib.h>
// #include "./go_fatfs.h"
import "C"
import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	gopointer "github.com/mattn/go-pointer"
)

const (
	FileResultOK                          = C.FR_OK /* (0) Succeeded */
	FileResultErr              FileResult = C.FR_DISK_ERR
	FileResultIntErr           FileResult = C.FR_INT_ERR
	FileResultNotReady         FileResult = C.FR_NOT_READY
	FileResultNoFile           FileResult = C.FR_NO_FILE
	FileResultNoPath           FileResult = C.FR_NO_PATH
	FileResultInvalidName      FileResult = C.FR_INVALID_NAME
	FileResultDenied           FileResult = C.FR_DENIED
	FileResultExist            FileResult = C.FR_EXIST
	FileResultInvalidObject    FileResult = C.FR_INVALID_OBJECT
	FileResultWriteProtected   FileResult = C.FR_WRITE_PROTECTED
	FileResultInvalidDrive     FileResult = C.FR_INVALID_DRIVE
	FileResultNotEnabled       FileResult = C.FR_NOT_ENABLED
	FileResultNoFilesystem     FileResult = C.FR_NO_FILESYSTEM
	FileResultMkfsAborted      FileResult = C.FR_MKFS_ABORTED
	FileResultTimeout          FileResult = C.FR_TIMEOUT
	FileResultLocked           FileResult = C.FR_LOCKED
	FileResultNotEnoughCore    FileResult = C.FR_NOT_ENOUGH_CORE
	FileResultTooManyOpenFiles FileResult = C.FR_TOO_MANY_OPEN_FILES
	FileResultInvalidParameter FileResult = C.FR_INVALID_PARAMETER
	FileResultReadOnly         FileResult = 99

	TypeFAT12 Type = C.FS_FAT12
	TypeFAT16 Type = C.FS_FAT16
	TypeFAT32 Type = C.FS_FAT32
	TypeEXFAT Type = C.FS_EXFAT

	AttrReadOnly  FileAttr = C.AM_RDO
	AttrHidden    FileAttr = C.AM_HID
	AttrSystem    FileAttr = C.AM_SYS
	AttrDirectory FileAttr = C.AM_DIR
	AttrArchive   FileAttr = C.AM_ARC

	SectorSize = 512

	FileAccessRead         OpenFlag = C.FA_READ
	FileAccessWrite        OpenFlag = C.FA_WRITE
	FileAccessOpenExisting OpenFlag = C.FA_OPEN_EXISTING
	FileAccessCreateNew    OpenFlag = C.FA_CREATE_NEW
	FileAccessCreateAlways OpenFlag = C.FA_CREATE_ALWAYS
	FileAccessOpenAlways   OpenFlag = C.FA_OPEN_ALWAYS
	FileAccessOpenAppend   OpenFlag = C.FA_OPEN_APPEND
)

type OpenFlag uint

type Type uint

func (t Type) String() string {
	switch t {
	case TypeFAT12:
		return "FAT12"
	case TypeFAT16:
		return "FAT16"
	case TypeFAT32:
		return "FAT32"
	case TypeEXFAT:
		return "EXFAT"
	default:
		return "invalid/unknown"
	}
}

type FileResult uint

func (r FileResult) Error() string {
	var msg string
	switch r {
	case FileResultErr:
		msg = "(1) A hard error occurred in the low level disk I/O layer"
	case FileResultIntErr:
		msg = "(2) Assertion failed"
	case FileResultNotReady:
		msg = "(3) The physical drive cannot work"
	case FileResultNoFile:
		msg = "(4) Could not find the file"
	case FileResultNoPath:
		msg = "(5) Could not find the path"
	case FileResultInvalidName:
		msg = "(6) The path name format is invalid"
	case FileResultDenied:
		msg = "(7) Access denied due to prohibited access or directory full"
	case FileResultExist:
		msg = "(8) Access denied due to prohibited access"
	case FileResultInvalidObject:
		msg = "(9) The file/directory object is invalid"
	case FileResultWriteProtected:
		msg = "(10) The physical drive is write protected"
	case FileResultInvalidDrive:
		msg = "(11) The logical drive number is invalid"
	case FileResultNotEnabled:
		msg = "(12) The volume has no work area"
	case FileResultNoFilesystem:
		msg = "(13) There is no valid FAT volume"
	case FileResultMkfsAborted:
		msg = "(14) The f_mkfs() aborted due to any problem"
	case FileResultTimeout:
		msg = "(15) Could not get a grant to access the volume within defined period"
	case FileResultLocked:
		msg = "(16) The operation is rejected according to the file sharing policy"
	case FileResultNotEnoughCore:
		msg = "(17) LFN working buffer could not be allocated"
	case FileResultTooManyOpenFiles:
		msg = "(18) Number of open files > FF_FS_LOCK"
	case FileResultInvalidParameter:
		msg = "(19) Given parameter is invalid"
	case FileResultReadOnly:
		msg = "(99) Read-only filesystem"
	default:
		msg = "unknown file result error"
	}
	return "fatfs: " + msg
}

type FileAttr byte

type Info struct {
	size int64
	name string
	attr FileAttr
}

var _ os.FileInfo = (*Info)(nil)

func (info *Info) Name() string {
	return info.name
}

func (info *Info) Size() int64 {
	return info.size
}

func (info *Info) IsDir() bool {
	return (info.attr & AttrDirectory) > 0
}

func (info *Info) Sys() interface{} {
	return nil
}

func (info *Info) Mode() os.FileMode {
	v := os.FileMode(0777)
	if info.IsDir() {
		v |= os.ModeDir
	}
	return v
}

func (info *Info) ModTime() time.Time {
	return time.Time{}
}

type FATFS struct {
	fs  *C.FATFS
	dev BlockDevice
}

type Config struct {
	SectorSize int
}

func New(blockdev BlockDevice) *FATFS {
	fs := &FATFS{
		fs:  C.go_fatfs_new_fatfs(),
		dev: blockdev,
	}
	fs.fs.drv = gopointer.Save(fs)
	return fs
}

func (l *FATFS) Mount() error {
	return errval(C.f_mount(l.fs))
}

func (l *FATFS) Format() error {
	work := make([]byte, SectorSize)
	return errval(C.f_mkfs(l.fs, C.FM_FAT, 0, unsafe.Pointer(&work[0]), C.UINT(len(work))))
}

func (l *FATFS) Free() (int64, error) {
	var clust C.DWORD
	res := C.f_getfree(l.fs, &clust)
	if err := errval(res); err != nil {
		return 0, err
	}
	return int64(clust * SectorSize), nil
}

func (l *FATFS) Unmount() error {
	return nil
}

func (l *FATFS) Remove(path string) error {
	cs := cstring(path)
	defer C.free(unsafe.Pointer(cs))
	return errval(C.f_unlink(l.fs, cs))
}

func (l *FATFS) Rename(oldPath string, newPath string) error {
	cs1, cs2 := cstring(oldPath), cstring(newPath)
	defer C.free(unsafe.Pointer(cs1))
	defer C.free(unsafe.Pointer(cs2))
	return errval(C.f_rename(l.fs, cs1, cs2))
}

func (l *FATFS) Stat(path string) (*Info, error) {
	cs := cstring(path)
	defer C.free(unsafe.Pointer(cs))
	info := C.FILINFO{}
	if err := errval(C.f_stat(l.fs, cs, &info)); err != nil {
		return nil, err
	}
	return &Info{
		size: int64(info.fsize),
		name: gostring(&info.fname[0]),
		attr: FileAttr(info.fattrib),
	}, nil
}

func (l *FATFS) Mkdir(path string) error {
	cs := cstring(path)
	defer C.free(unsafe.Pointer(cs))
	return errval(C.f_mkdir(l.fs, cs))
}

func (l *FATFS) Open(path string) (*File, error) {
	return l.OpenFile(path, FileAccessRead)
}

func (l *FATFS) OpenFile(path string, flags OpenFlag) (*File, error) {

	// TODO: better handling for paths
	//for strings.HasPrefix(path, "/") {
	//	path = strings.TrimLeft(path, "/")
	//}
	//println("opening:", path)

	// create a C string with the file path
	cs := cstring(path)
	defer C.free(unsafe.Pointer(cs))

	// stat the file path to see if it exists and if it is a file/dir
	info := &C.FILINFO{}
	if err := errval(C.f_stat(l.fs, cs, info)); err != nil && err != FileResultNoFile && err != FileResultInvalidName {
		//println("warning:", err)
		return nil, err
	}

	// use f_open or f_opendir to obtain a handle to the object
	var file = &File{fs: l, name: path}
	var errno C.FRESULT
	if path == "/" || info.fattrib&C.AM_DIR > 0 {
		// directory
		file.typ = uint8(C.AM_DIR)
		file.hndl = unsafe.Pointer(C.go_fatfs_new_ff_dir())
		errno = C.f_opendir(l.fs, (*C.FF_DIR)(file.hndl), cs)
	} else {
		// file
		file.typ = 0
		file.hndl = unsafe.Pointer(C.go_fatfs_new_fil())
		errno = C.f_open(l.fs, (*C.FIL)(file.hndl), cs, C.BYTE(flags))
	}

	// check to make sure f_open/f_opendir didn't produce an error
	if err := errval(errno); err != nil {
		if file.hndl != nil {
			C.free(file.hndl)
			file.hndl = nil
		}
		return nil, err
	}

	// file handle was initialized successfully
	return file, nil
}

type File struct {
	fs   *FATFS
	typ  uint8
	hndl unsafe.Pointer
	name string
}

func (f *File) dirptr() *C.FF_DIR {
	return (*C.FF_DIR)(f.hndl)
}

func (f *File) fileptr() *C.FIL {
	return (*C.FIL)(f.hndl)
}

// Name returns the name of the file as presented to OpenFile
func (f *File) Name() string {
	return f.name
}

func (f *File) Close() error {
	var errno C.FRESULT
	if f.hndl != nil {
		defer func() {
			C.free(f.hndl)
			f.hndl = nil
		}()
		if f.IsDir() {
			errno = C.f_closedir(f.dirptr())
		} else {
			errno = C.f_close(f.fileptr())
		}
	}
	return errval(errno)
}

func (f *File) Read(buf []byte) (n int, err error) {
	if f.IsDir() {
		return 0, FileResultInvalidObject
	}
	bufptr := unsafe.Pointer(&buf[0])
	var br, btr C.UINT = 0, C.UINT(len(buf))
	errno := C.f_read(f.fileptr(), bufptr, btr, &br)
	if err := errval(errno); err != nil {
		return int(br), err
	}
	if br == 0 && btr > 0 {
		return 0, io.EOF
	}
	return int(br), nil
}

/*
// Seek changes the position of the file
func (f *File) Seek(offset int64, whence int) (ret int64, err error) {
	errno := C.int(C.lfs_file_seek(f.lfs.lfs, &f.fptr, C.lfs_soff_t(offset), C.int(whence)))
	if errno < 0 {
		return -1, errval(errno)
	}
	return int64(errno), nil
}

// Tell returns the position of the file
func (f *File) Tell() (ret int64, err error) {
	errno := C.int(C.lfs_file_tell(f.lfs.lfs, &f.fptr))
	if errno < 0 {
		return -1, errval(errno)
	}
	return int64(errno), nil
}

// Rewind changes the position of the file to the beginning of the file
func (f *File) Rewind() (err error) {
	return errval(C.lfs_file_rewind(f.lfs.lfs, &f.fptr))
}
*/

// Size returns the size of the file
func (f *File) Size() (int64, error) {
	if f.IsDir() {
		ptr := f.dirptr()
		return int64(ptr.obj.objsize), nil
	} else {
		ptr := f.fileptr()
		return int64(ptr.obj.objsize), nil
	}
}

// Synchronize a file on storage
//
// Any pending writes are written out to storage.
// Returns a negative error code on failure.
func (f *File) Sync() error {
	return errval(C.f_sync(f.fileptr()))
}

/*
// Truncates the size of the file to the specified size
//
// Returns a negative error code on failure.
func (f *File) Truncate(size uint32) error {
	return errval(C.lfs_file_truncate(f.lfs.lfs, &f.fptr, C.lfs_off_t(size)))
}
*/

func (f *File) Write(buf []byte) (n int, err error) {
	if f.IsDir() {
		return 0, FileResultInvalidObject
	}
	bufptr := unsafe.Pointer(&buf[0])
	var bw, btw C.UINT = 0, C.UINT(len(buf))
	errno := C.f_write(f.fileptr(), bufptr, btw, &bw)
	if err := errval(errno); err != nil {
		return int(bw), err
	}
	if bw < btw {
		return int(bw), errors.New("volume is full")
	}
	return int(bw), nil
}

func (f *File) IsDir() bool {
	return f.typ == C.AM_DIR
}

func (f *File) Readdir(n int) (infos []os.FileInfo, err error) {
	if !f.IsDir() {
		return nil, FileResultInvalidObject
	}
	if n == 0 {
		// passing nil pointer to f_readdir resets the read index
		if err := errval(C.f_readdir(f.dirptr(), nil)); err != nil {
			return nil, err
		}
	}
	for {
		info := C.FILINFO{}
		if err := errval(C.f_readdir(f.dirptr(), &info)); err != nil {
			return nil, err
		}
		if fname := gostring(&info.fname[0]); fname == "" {
			return infos, nil
		} else {
			infos = append(infos, &Info{
				size: int64(info.fsize),
				name: fname,
				attr: FileAttr(info.fattrib),
			})
		}
	}
}

// would be nice to use C.CString instead, but TinyGo doesn't seem to support
func cstring(s string) *C.char {
	ptr := C.malloc(C.size_t(len(s) + 1))
	buf := (*[1 << 28]byte)(ptr)[: len(s)+1 : len(s)+1]
	copy(buf, s)
	buf[len(s)] = 0
	return (*C.char)(ptr)
}

// would be nice to use C.GoString instead, but TinyGo doesn't seem to support
func gostring(s *C.char) string {
	slen := int(C.strlen(s))
	sbuf := make([]byte, slen)
	copy(sbuf, (*[1 << 28]byte)(unsafe.Pointer(s))[:slen:slen])
	return string(sbuf)
}

func errval(errno C.FRESULT) error {
	if errno > FileResultOK {
		return FileResult(errno)
	}
	return nil
}

// A BlockDevice is the raw device that is meant to store a filesystem.
type BlockDevice interface {

	// ReadAt reads the given number of bytes from the block device.
	io.ReaderAt

	// WriteAt writes the given number of bytes to the block device.
	io.WriterAt

	// Size returns the number of bytes in this block device.
	Size() int64

	// SectorSize returns the size of a single sector on this device.
	SectorSize() int64

	// EraseBlockSize returns the smallest erasable area on this particular chip
	// in bytes. This is used for the block size in EraseBlocks.
	// It must be a power of two, and may be as small as 1. A typical size is 4096.
	EraseBlockSize() int64

	// EraseBlocks erases the given number of blocks. An implementation may
	// transparently coalesce ranges of blocks into larger bundles if the chip
	// supports this. The start and len parameters are in block numbers, use
	// EraseBlockSize to map addresses to blocks.
	EraseBlocks(start, len int64) error

	// Sync triggers the devices to commit any pending or cached operations
	Sync() error
}

func xxdfprint(w io.Writer, offset uint32, b []byte) {
	var l int
	var buf16 = make([]byte, 16)
	var padding = ""
	for i, c := 0, len(b); i < c; i += 16 {
		l = i + 16
		if l >= c {
			padding = strings.Repeat(" ", (l-c)*3)
			l = c
		}
		fmt.Fprintf(w, "%08x: % x    "+padding, offset+uint32(i), b[i:l])
		for j, n := 0, l-i; j < 16; j++ {
			if j >= n {
				buf16[j] = ' '
			} else if !strconv.IsPrint(rune(b[i+j])) {
				buf16[j] = '.'
			} else {
				buf16[j] = b[i+j]
			}
		}
		w.Write(buf16)
		fmt.Fprintln(w)
	}
}
