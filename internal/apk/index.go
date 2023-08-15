package apk

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

type apkindex struct {
	checksum []byte
	name     string
	version  string
}

type pkginfo struct {
	origin string
	commit string
}

func (h *handler) renderIndex(w http.ResponseWriter, r *http.Request, in io.Reader, ref string) error {
	short := r.URL.Query().Get("short") != "false"

	pkgs := []apkindex{}
	ptov := map[string]string{}

	if err := headerTmpl.Execute(w, TitleData{title(ref)}); err != nil {
		return err
	}
	header := headerData(ref, v1.Descriptor{})
	before, _, ok := strings.Cut(ref, "@")
	if ok {
		u := "https://" + strings.TrimSuffix(strings.TrimPrefix(before, "/https/"), "/")
		if short {
			// Link to long form.
			header.JQ = "curl" + " " + u + ` | tar -Oxz <a class="mt" href="?short=false">APKINDEX</a>`

			// awk -F':' '/^P:/{printf "%s-", $2} /^V:/{printf "%s.apk\n", $2}'
			header.JQ += ` | awk -F':' '/^P:/{printf "%s-", $2} /^V:/{printf "%s.apk\n", $2}'`
		} else {
			header.JQ = "curl" + " " + u + " | tar -Oxz APKINDEX"
		}
	} else if before, _, ok := strings.Cut(ref, "APKINDEX.tar.gz"); ok {
		before = path.Join(before, "APKINDEX.tar.gz")
		u := "https://" + strings.TrimSuffix(strings.TrimPrefix(before, "/https/"), "/")
		if short {
			// Link to long form.
			header.JQ = "curl" + " " + u + ` | tar -Oxz <a class="mt" href="?short=false">APKINDEX</a>`

			// awk -F':' '/^P:/{printf "%s-", $2} /^V:/{printf "%s.apk\n", $2}'
			header.JQ += ` | awk -F':' '/^P:/{printf "%s-", $2} /^V:/{printf "%s.apk\n", $2}'`
		} else {
			header.JQ = "curl" + " " + u + " | tar -Oxz APKINDEX"
		}
	}

	if err := bodyTmpl.Execute(w, header); err != nil {
		return err
	}

	fmt.Fprintf(w, "<pre><div>")

	scanner := bufio.NewScanner(bufio.NewReaderSize(in, 1<<16))

	prefix, _, ok := strings.Cut(r.URL.Path, "APKINDEX.tar.gz")
	if !ok {
		return fmt.Errorf("something funky with path...")
	}

	pkg := apkindex{}

	for scanner.Scan() {
		line := scanner.Text()

		before, after, ok := strings.Cut(line, ":")
		if !ok {
			// reset pkg
			pkg = apkindex{}

			if !short {
				fmt.Fprintf(w, "</div><div>\n")
			}

			continue
		}

		switch before {
		case "C":
			chk := strings.TrimPrefix(after, "Q1")
			decoded, err := base64.StdEncoding.DecodeString(chk)
			if err != nil {
				return fmt.Errorf("base64 decode: %w", err)
			}

			pkg.checksum = decoded
		case "P":
			pkg.name = after
		case "V":
			pkg.version = after
		}

		if short {
			if before == "V" {
				ptov[pkg.name] = pkg.version
				pkgs = append(pkgs, pkg)
			}
			continue
		}

		switch before {
		case "V":
			apk := fmt.Sprintf("%s-%s.apk", pkg.name, pkg.version)
			hexsum := "sha1:" + hex.EncodeToString(pkg.checksum)
			href := fmt.Sprintf("%s@%s", path.Join(prefix, apk), hexsum)
			fmt.Fprintf(w, "<a id=%q href=%q>V:%s</a>\n", apk, href, pkg.version)
		case "S", "I":
			i, err := strconv.ParseInt(after, 10, 64)
			if err != nil {
				return fmt.Errorf("parsing %q as int: %w", after, err)
			}
			fmt.Fprintf(w, "%s:<span title=%q>%s</span>\n", before, humanize.Bytes(uint64(i)), after)
		case "t":
			sec, err := strconv.ParseInt(after, 10, 64)
			if err != nil {
				return fmt.Errorf("parsing %q as timestamp: %w", after, err)
			}
			t := time.Unix(sec, 0)
			fmt.Fprintf(w, "<span title=%q>t:%s</span>\n", t.String(), after)
		default:
			fmt.Fprintf(w, "%s\n", line)
		}
	}

	for _, pkg := range pkgs {
		last, ok := ptov[pkg.name]
		if !ok {
			return fmt.Errorf("did not see %q", pkg.name)
		}

		bold := pkg.version == last

		apk := fmt.Sprintf("%s-%s.apk", pkg.name, pkg.version)
		hexsum := "sha1:" + hex.EncodeToString(pkg.checksum)
		href := fmt.Sprintf("%s@%s", path.Join(prefix, apk), hexsum)

		if !bold {
			fmt.Fprintf(w, "<a class=%q href=%q>%s</a>\n", "mt", href, apk)
		} else {
			fmt.Fprintf(w, "<a href=%q>%s</a>\n", href, apk)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner: %w", err)
	}

	fmt.Fprintf(w, "</div></pre>\n</body>\n</html>\n")

	return nil
}

func (h *handler) renderPkgInfo(w http.ResponseWriter, r *http.Request, in io.Reader, ref string) error {
	if err := headerTmpl.Execute(w, TitleData{title(ref)}); err != nil {
		return err
	}
	header := headerData(ref, v1.Descriptor{})
	before, _, ok := strings.Cut(ref, "@")
	if ok {
		u := "https://" + strings.TrimSuffix(strings.TrimPrefix(before, "/https/"), "/")
		header.JQ = "curl" + " " + u + " | tar -Oxz .PKGINFO"
	}

	if err := bodyTmpl.Execute(w, header); err != nil {
		return err
	}

	fmt.Fprintf(w, "<pre><div>")

	scanner := bufio.NewScanner(bufio.NewReaderSize(in, 1<<16))

	pkg := pkginfo{}

	for scanner.Scan() {
		line := scanner.Text()

		before, after, ok := strings.Cut(line, "=")
		if !ok {

			fmt.Fprintf(w, "</div><div>\n")

			continue
		}

		before = strings.TrimSpace(before)
		after = strings.TrimSpace(after)

		switch before {
		case "origin":
			pkg.origin = after
		case "commit":
			pkg.commit = after
		}

		switch before {
		case "commit":
			if !strings.Contains(r.URL.Path, "packages.wolfi.dev") {
				fmt.Fprintf(w, "%s\n", line)
				continue
			}

			href := fmt.Sprintf("https://github.com/wolfi-dev/os/blob/%s/%s.yaml", pkg.commit, pkg.origin)
			fmt.Fprintf(w, "%s = <a href=%q>%s</a>\n", before, href, after)
		case "size":
			i, err := strconv.ParseInt(after, 10, 64)
			if err != nil {
				return fmt.Errorf("parsing %q as int: %w", after, err)
			}
			fmt.Fprintf(w, "%s = <span title=%q>%s</span>\n", before, humanize.Bytes(uint64(i)), after)
		case "builddate":
			sec, err := strconv.ParseInt(after, 10, 64)
			if err != nil {
				return fmt.Errorf("parsing %q as timestamp: %w", after, err)
			}
			t := time.Unix(sec, 0)
			fmt.Fprintf(w, "before = <span title=%q>%s</span>\n", t.String(), after)
		default:
			fmt.Fprintf(w, "%s\n", line)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner: %w", err)
	}

	fmt.Fprintf(w, "</div></pre>\n</body>\n</html>\n")

	return nil
}
