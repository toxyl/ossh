package main

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// DirExists reports whether the dir exists as a boolean,
// taken from https://stackoverflow.com/a/49697453 / https://stackoverflow.com/a/51870143/3337885
func DirExists(name string) bool {
	fileOrDir, err := os.Open(name)
	if err != nil {
		return false
	}
	defer fileOrDir.Close()

	info, err := fileOrDir.Stat()
	if err != nil {
		return false
	}
	if info.IsDir() {
		return true
	}
	return false
}

func FileExists(name string) bool {
	file, err := os.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()

	_, err = file.Stat()
	return err == nil
}

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

var floatType = reflect.TypeOf(float64(0))
var stringType = reflect.TypeOf("")

func GetFloat(val interface{}) (float64, error) {
	switch i := val.(type) {
	case float64:
		return i, nil
	case float32:
		return float64(i), nil
	case int64:
		return float64(i), nil
	case int32:
		return float64(i), nil
	case int:
		return float64(i), nil
	case uint64:
		return float64(i), nil
	case uint32:
		return float64(i), nil
	case uint:
		return float64(i), nil
	case string:
		return strconv.ParseFloat(i, 64)
	default:
		v := reflect.ValueOf(val)
		v = reflect.Indirect(v)
		if v.Type().ConvertibleTo(floatType) {
			fv := v.Convert(floatType)
			return fv.Float(), nil
		} else if v.Type().ConvertibleTo(stringType) {
			sv := v.Convert(stringType)
			s := sv.String()
			return strconv.ParseFloat(s, 64)
		}
		return math.NaN(), fmt.Errorf("Can't convert %v to float64", v.Type())
	}
}

func StringToInt64(s string, defaultValue int64) int64 {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		i = defaultValue
	}
	return i
}

func StringToInt32(s string, defaultValue int32) int32 {
	i, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		i = int64(defaultValue)
	}
	return int32(i)
}

func StringToInt(s string, defaultValue int) int {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		i = int64(defaultValue)
	}
	return int(i)
}

func StringToSha256(s string) string {
	data := []byte(s)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:])
}

func StringToSha1(s string) string {
	data := []byte(s)
	hash := sha1.Sum(data)
	return fmt.Sprintf("%x", hash[:])
}

func StringSliceToSha256(s []string) string {
	sort.Strings(s)
	return StringToSha256(strings.Join(s, ","))
}

func ImplodeLines(lines []string) string {
	return strings.Join(lines, "\n")
}

func ExplodeLines(lines string) []string {
	return strings.Split(lines, "\n")
}

// StringSliceDifference returns the elements in `a` that aren't in `b`.
// from https://stackoverflow.com/a/45428032/3337885
func StringSliceDifference(a, b []string) []string {
	mb := make(map[string]struct{}, len(b))
	for _, x := range b {
		mb[x] = struct{}{}
	}
	var diff []string
	for _, x := range a {
		if _, found := mb[x]; !found {
			diff = append(diff, x)
		}
	}
	return diff
}

// from https://stackoverflow.com/a/37897238/3337885
func RealAddr(r *http.Request) string {
	remoteIP := ""
	// the default is the originating ip. but we try to find better options because this is almost
	// never the right IP
	if parts := strings.Split(r.RemoteAddr, ":"); len(parts) == 2 {
		remoteIP = parts[0]
	}
	// If we have a forwarded-for header, take the address from there
	if xff := strings.Trim(r.Header.Get("X-Forwarded-For"), ","); len(xff) > 0 {
		addrs := strings.Split(xff, ",")
		lastFwd := addrs[len(addrs)-1]
		if ip := net.ParseIP(lastFwd); ip != nil {
			remoteIP = ip.String()
		}
		// parse X-Real-Ip header
	} else if xri := r.Header.Get("X-Real-Ip"); len(xri) > 0 {
		if ip := net.ParseIP(xri); ip != nil {
			remoteIP = ip.String()
		}
	}

	return remoteIP
}

func EncodeBase64String(src string) string {
	res := base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(src)))
	return res
}

func DecodeBase64String(src string) (string, error) {
	srcdec, err := base64.StdEncoding.DecodeString(src)
	if err != nil {
		return "", err
	}
	res := strings.TrimSpace(string(srcdec))
	return res, nil
}
