package gosseract

// #if __FreeBSD__ >= 10
// #cgo LDFLAGS: -L/usr/local/lib -llept -ltesseract
// #else
// #cgo LDFLAGS: -llept -ltesseract
// #endif
// #include <stdlib.h>
// #include <stdbool.h>
// #include "tessbridge.h"
import "C"
import (
	"fmt"
	"os"
	"strings"
	"unsafe"
)

// Version returns the version of Tesseract-OCR
func Version() string {
	api := C.Create()
	defer C.Free(api)
	version := C.Version(api)
	return C.GoString(version)
}

// Client is argument builder for tesseract::TessBaseAPI.
type Client struct {
	api C.TessBaseAPI

	// Trim specifies characters to trim, which would be trimed from result string.
	// As results of OCR, text often contains unnecessary characters, such as newlines, on the head/foot of string.
	// If `Trim` is set, this client will remove specified characters from the result.
	Trim bool

	// TessdataPrefix can indicate directory path to `tessdata`.
	// It is set `/usr/local/share/tessdata/` or something like that, as default.
	// TODO: Implement and test
	TessdataPrefix *string

	// Languages are languages to be detected. If not specified, it's gonna be "eng".
	Languages []string

	// ImagePath is just path to image file to be processed OCR.
	ImagePath string

	// Variables is just a pool to evaluate "tesseract::TessBaseAPI->SetVariable" in delay.
	// TODO: Think if it should be public, or private property.
	Variables map[string]string

	// PageSegMode is a mode for page layout analysis.
	// See https://github.com/otiai10/gosseract/issues/52 for more information.
	PageSegMode *PageSegMode

	// Config is a file path to the configuration for Tesseract
	// See http://www.sk-spell.sk.cx/tesseract-ocr-parameters-in-302-version
	// TODO: Fix link to official page
	ConfigFilePath string
}

// NewClient construct new Client. It's due to caller to Close this client.
func NewClient() *Client {
	client := &Client{
		api:       C.Create(),
		Variables: map[string]string{},
		Trim:      true,
	}
	return client
}

// Close frees allocated API. This MUST be called for ANY client constructed by "NewClient" function.
func (c *Client) Close() (err error) {
	// defer func() {
	// 	if e := recover(); e != nil {
	// 		err = fmt.Errorf("%v", e)
	// 	}
	// }()
	C.Free(c.api)
	return err
}

// SetImage sets path to image file to be processed OCR.
func (c *Client) SetImage(imagepath string) *Client {
	c.ImagePath = imagepath
	return c
}

// SetLanguage sets languages to use. English as default.
func (c *Client) SetLanguage(langs ...string) *Client {
	c.Languages = langs
	return c
}

// SetWhitelist sets whitelist chars.
// See official documentation for whitelist here https://github.com/tesseract-ocr/tesseract/wiki/ImproveQuality#dictionaries-word-lists-and-patterns
func (c *Client) SetWhitelist(whitelist string) *Client {
	return c.SetVariable("tessedit_char_whitelist", whitelist)
}

// SetVariable sets parameters, representing tesseract::TessBaseAPI->SetVariable.
// See official documentation here https://zdenop.github.io/tesseract-doc/classtesseract_1_1_tess_base_a_p_i.html#a2e09259c558c6d8e0f7e523cbaf5adf5
func (c *Client) SetVariable(key, value string) *Client {
	c.Variables[key] = value
	return c
}

// SetPageSegMode sets "Page Segmentation Mode" (PSM) to detect layout of characters.
// See official documentation for PSM here https://github.com/tesseract-ocr/tesseract/wiki/ImproveQuality#page-segmentation-method
func (c *Client) SetPageSegMode(mode PageSegMode) *Client {
	c.PageSegMode = &mode
	return c
}

// SetConfigFile sets the file path to config file.
func (c *Client) SetConfigFile(fpath string) error {
	info, err := os.Stat(fpath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("the specified config file path seems to be a directory")
	}
	c.ConfigFilePath = fpath
	return nil
}

// It's due to the caller to free this char pointer.
func (c *Client) charLangs() *C.char {
	var langs *C.char
	if len(c.Languages) != 0 {
		langs = C.CString(strings.Join(c.Languages, "+"))
	}
	return langs
}

// It's due to the caller to free this char pointer.
func (c *Client) charConfig() *C.char {
	var config *C.char
	if _, err := os.Stat(c.ConfigFilePath); err == nil {
		config = C.CString(c.ConfigFilePath)
	}
	return config
}

// Initialize tesseract::TessBaseAPI
// TODO: add tessdata prefix
func (c *Client) init() error {
	langs := c.charLangs()
	defer C.free(unsafe.Pointer(langs))
	config := c.charConfig()
	defer C.free(unsafe.Pointer(config))
	res := C.Init(c.api, nil, langs, config)
	if res != 0 {
		// TODO: capture and vacuum stderr from Cgo
		return fmt.Errorf("failed to initialize TessBaseAPI with code %d", res)
	}
	return nil
}

// Prepare tesseract::TessBaseAPI options,
// must be called after `init`.
func (c *Client) prepare() error {
	// Set Image by giving path
	imagepath := C.CString(c.ImagePath)
	defer C.free(unsafe.Pointer(imagepath))
	C.SetImage(c.api, imagepath)

	for key, value := range c.Variables {
		if ok := c.bind(key, value); !ok {
			return fmt.Errorf("failed to set variable with key(%s):value(%s)", key, value)
		}
	}

	if c.PageSegMode != nil {
		mode := C.int(*c.PageSegMode)
		C.SetPageSegMode(c.api, mode)
	}
	return nil
}

// Binds variable to API object.
// Must be called from inside `prepare`.
func (c *Client) bind(key, value string) bool {
	k, v := C.CString(key), C.CString(value)
	defer C.free(unsafe.Pointer(k))
	defer C.free(unsafe.Pointer(v))
	res := C.SetVariable(c.api, k, v)
	return bool(res)
}

// Text finally initialize tesseract::TessBaseAPI, execute OCR and extract text detected as string.
func (c *Client) Text() (out string, err error) {
	if err = c.init(); err != nil {
		return
	}
	if err = c.prepare(); err != nil {
		return
	}
	out = C.GoString(C.UTF8Text(c.api))
	if c.Trim {
		out = strings.Trim(out, "\n")
	}
	return out, err
}

// HTML finally initialize tesseract::TessBaseAPI, execute OCR and returns hOCR text.
// See https://en.wikipedia.org/wiki/HOCR for more information of hOCR.
func (c *Client) HTML() (out string, err error) {
	if err = c.init(); err != nil {
		return
	}
	if err = c.prepare(); err != nil {
		return
	}
	out = C.GoString(C.HOCRText(c.api))
	return
}
