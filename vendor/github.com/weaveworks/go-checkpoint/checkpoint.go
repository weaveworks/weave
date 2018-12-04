// Package checkpoint is a package for checking version information and alerts
// for a Weaveworks product.
package checkpoint

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/scrypt"
	"io"
	"io/ioutil"
	mrand "math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-cleanhttp"
)

var magicBytes = [4]byte{0x35, 0x77, 0x69, 0xFB}

// CheckParams are the parameters for configuring a check request.
type CheckParams struct {
	// Product and version are used to lookup the correct product and
	// alerts for the proper version. The version is also used to perform
	// a version check.
	Product string
	Version string

	// Generic product flags
	Flags      map[string]string
	ExtraFlags func() []Flag

	// Arch and OS are used to filter alerts potentially only to things
	// affecting a specific os/arch combination. If these aren't specified,
	// they'll be automatically filled in.
	Arch string
	OS   string

	// Signature is some random signature that should be stored and used
	// as a cookie-like value. This ensures that alerts aren't repeated.
	// If the signature is changed, repeat alerts may be sent down. The
	// signature should NOT be anything identifiable to a user (such as
	// a MAC address). It should be random.
	//
	// If SignatureFile is given, then the signature will be read from this
	// file. If the file doesn't exist, then a random signature will
	// automatically be generated and stored here. SignatureFile will be
	// ignored if Signature is given.
	Signature     string
	SignatureFile string

	// CacheFile, if specified, will cache the result of a check. The
	// duration of the cache is specified by CacheDuration, and defaults
	// to 48 hours if not specified. If the CacheFile is newer than the
	// CacheDuration, than the Check will short-circuit and use those
	// results.
	//
	// If the CacheFile directory doesn't exist, it will be created with
	// permissions 0755.
	CacheFile     string
	CacheDuration time.Duration

	// Force, if true, will force the check even if CHECKPOINT_DISABLE
	// is set. Within HashiCorp products, this is ONLY USED when the user
	// specifically requests it. This is never automatically done without
	// the user's consent.
	Force bool
}

// CheckResponse is the response for a check request.
type CheckResponse struct {
	Product             string
	CurrentVersion      string `json:"current_version"`
	CurrentReleaseDate  int    `json:"current_release_date"`
	CurrentDownloadURL  string `json:"current_download_url"`
	CurrentChangelogURL string `json:"current_changelog_url"`
	ProjectWebsite      string `json:"project_website"`
	Outdated            bool   `json:"outdated"`
	Alerts              []*CheckAlert
}

// CheckAlert is a single alert message from a check request.
//
// These never have to be manually constructed, and are typically populated
// into a CheckResponse as a result of the Check request.
type CheckAlert struct {
	ID      int
	Date    int
	Message string
	URL     string
	Level   string
}

// Checker is a state of a checker.
type Checker struct {
	doneCh          chan struct{}
	nextCheckAt     time.Time
	nextCheckAtLock sync.RWMutex
}

// Flag is some extra information about a product we want to pass along.
type Flag struct {
	Key   string
	Value string
}

// Check checks for alerts and new version information.
func Check(p *CheckParams) (*CheckResponse, error) {
	if IsCheckDisabled() && !p.Force {
		return &CheckResponse{}, nil
	}

	// If we have a cached result, then use that
	if r, err := checkCache(p.Version, p.CacheFile, p.CacheDuration); err != nil {
		return nil, err
	} else if r != nil {
		defer r.Close()
		return checkResult(r)
	}

	var u url.URL

	if p.Arch == "" {
		p.Arch = runtime.GOARCH
	}
	if p.OS == "" {
		p.OS = runtime.GOOS
	}

	// If we're not given a Signature, then attempt to read one from a
	// file, if specified, or derive it from the system uuid.
	//
	// NB: We ignore errors here since it is better to perform the
	// check with an empty signature than not at all.
	var signature string
	switch {
	case p.Signature != "":
		signature = p.Signature
	case p.SignatureFile != "":
		signature, _ = checkSignature(p.SignatureFile)
	default:
		signature, _ = getSystemUUID()
	}

	v := u.Query()
	v.Set("version", p.Version)
	v.Set("arch", p.Arch)
	v.Set("os", p.OS)
	v.Set("signature", signature)
	const flagPrefix = "flag_"
	for flag, value := range p.Flags {
		v.Set(flagPrefix+flag, value)
	}
	if p.ExtraFlags != nil {
		for _, d := range p.ExtraFlags() {
			v.Add(flagPrefix+d.Key, d.Value)
		}
	}

	u.Scheme = "https"
	u.Host = "checkpoint-api.weave.works"
	u.Path = fmt.Sprintf("/v1/check/%s", p.Product)
	u.RawQuery = v.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", "HashiCorp/go-checkpoint")

	client := cleanhttp.DefaultClient()
	defer client.Transport.(*http.Transport).CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Unknown status: %d", resp.StatusCode)
	}

	var r io.Reader = resp.Body
	defer resp.Body.Close()
	if p.CacheFile != "" {
		// Make sure the directory holding our cache exists.
		if err := os.MkdirAll(filepath.Dir(p.CacheFile), 0755); err != nil {
			return nil, err
		}

		// We have to cache the result, so write the response to the
		// file as we read it.
		f, err := os.Create(p.CacheFile)
		if err != nil {
			return nil, err
		}

		// Write the cache header
		if err := writeCacheHeader(f, p.Version); err != nil {
			f.Close()
			os.Remove(p.CacheFile)
			return nil, err
		}

		defer f.Close()
		r = io.TeeReader(r, f)
	}

	return checkResult(r)
}

// CheckInterval is used to check for a response on a given interval duration.
// The interval is not exact, and checks are randomized to prevent a thundering
// herd. However, it is expected that on average one check is performed per
// interval.
// The first check happens immediately after a goroutine which is responsible for
// making checks has been started.
func CheckInterval(p *CheckParams, interval time.Duration,
	cb func(*CheckResponse, error)) *Checker {

	state := &Checker{
		doneCh: make(chan struct{}),
	}

	if IsCheckDisabled() {
		return state
	}

	go func() {
		cb(Check(p))

		for {
			after := randomStagger(interval)
			state.nextCheckAtLock.Lock()
			state.nextCheckAt = time.Now().Add(after)
			state.nextCheckAtLock.Unlock()

			select {
			case <-time.After(after):
				cb(Check(p))
			case <-state.doneCh:
				return
			}
		}
	}()

	return state
}

// NextCheckAt returns at what time next check will happen.
func (c *Checker) NextCheckAt() time.Time {
	c.nextCheckAtLock.RLock()
	defer c.nextCheckAtLock.RUnlock()
	return c.nextCheckAt
}

// Stop stops the checker.
func (c *Checker) Stop() {
	close(c.doneCh)
}

// IsCheckDisabled returns true if checks are disabled.
func IsCheckDisabled() bool {
	return os.Getenv("CHECKPOINT_DISABLE") != ""
}

// randomStagger returns an interval that is between 3/4 and 5/4 of
// the given interval. The expected value is the interval.
func randomStagger(interval time.Duration) time.Duration {
	stagger := time.Duration(mrand.Int63()) % (interval / 2)
	return 3*(interval/4) + stagger
}

func checkCache(current string, path string, d time.Duration) (io.ReadCloser, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, not a problem
			return nil, nil
		}

		return nil, err
	}

	if d == 0 {
		d = 48 * time.Hour
	}

	if fi.ModTime().Add(d).Before(time.Now()) {
		// Cache is busted, delete the old file and re-request. We ignore
		// errors here because re-creating the file is fine too.
		os.Remove(path)
		return nil, nil
	}

	// File looks good so far, open it up so we can inspect the contents.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	// Check the signature of the file
	var sig [4]byte
	if err := binary.Read(f, binary.LittleEndian, sig[:]); err != nil {
		f.Close()
		return nil, err
	}
	if !reflect.DeepEqual(sig, magicBytes) {
		// Signatures don't match. Reset.
		f.Close()
		return nil, nil
	}

	// Check the version. If it changed, then rewrite
	var length uint32
	if err := binary.Read(f, binary.LittleEndian, &length); err != nil {
		f.Close()
		return nil, err
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(f, data); err != nil {
		f.Close()
		return nil, err
	}
	if string(data) != current {
		// Version changed, reset
		f.Close()
		return nil, nil
	}

	return f, nil
}

func checkResult(r io.Reader) (*CheckResponse, error) {
	var result CheckResponse
	dec := json.NewDecoder(r)
	if err := dec.Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// getSystemUUID returns the base64 encoded, scrypt hashed contents of
// /sys/class/dmi/id/product_uuid, or, if that is not available,
// sys/hypervisor/uuid.
func getSystemUUID() (string, error) {
	uuid, err := ioutil.ReadFile("/sys/class/dmi/id/product_uuid")
	if os.IsNotExist(err) {
		uuid, err = ioutil.ReadFile("/sys/hypervisor/uuid")
	}
	if err != nil {
		return "", err
	}
	if len(uuid) <= 0 {
		return "", fmt.Errorf("Empty system uuid")
	}
	hash, err := scrypt.Key(uuid, uuid, 16384, 8, 1, 32)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(hash), nil
}

func checkSignature(path string) (string, error) {
	_, err := os.Stat(path)
	if err == nil {
		// The file exists, read it out
		sigBytes, err := ioutil.ReadFile(path)
		if err != nil {
			return "", err
		}

		// Split the file into lines
		lines := strings.SplitN(string(sigBytes), "\n", 2)
		if len(lines) > 0 {
			return strings.TrimSpace(lines[0]), nil
		}
	}

	// If this isn't a non-exist error, then return that.
	if !os.IsNotExist(err) {
		return "", err
	}

	// The file doesn't exist, so create a signature.
	var b [16]byte
	n := 0
	for n < 16 {
		n2, err := rand.Read(b[n:])
		if err != nil {
			return "", err
		}

		n += n2
	}
	signature := fmt.Sprintf(
		"%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])

	// Make sure the directory holding our signature exists.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}

	// Write the signature
	if err := ioutil.WriteFile(path, []byte(signature+"\n\n"+userMessage+"\n"), 0644); err != nil {
		return "", err
	}

	return signature, nil
}

func writeCacheHeader(f io.Writer, v string) error {
	// Write our signature first
	if err := binary.Write(f, binary.LittleEndian, magicBytes); err != nil {
		return err
	}

	// Write out our current version length
	var length = uint32(len(v))
	if err := binary.Write(f, binary.LittleEndian, length); err != nil {
		return err
	}

	_, err := f.Write([]byte(v))
	return err
}

// userMessage is suffixed to the signature file to provide feedback.
var userMessage = `
This signature is a randomly generated UUID used to de-duplicate
alerts and version information. This signature is random, it is
not based on any personally identifiable information. To create
a new signature, you can simply delete this file at any time.
See the documentation for the software using Checkpoint for more
information on how to disable it.
`
