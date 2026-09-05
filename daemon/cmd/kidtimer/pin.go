package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/pin"
	"kidtimer/daemon/internal/reverse"
)

func runPin(args []string) error {
	if len(args) < 1 || (args[0] != "set" && args[0] != "status") {
		return fmt.Errorf("usage: kidtimer pin set|status")
	}
	if args[0] == "status" {
		return runPinStatus(args[1:])
	}
	return runPinSet(args[1:])
}

func runPinStatus(args []string) error {
	fs := flag.NewFlagSet("pin status", flag.ContinueOnError)
	home := fs.String("home", "", "household dir (default ~/.local/share/kidtimer)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := shareDir(*home)
	if err != nil {
		return err
	}
	_, ok, err := household.ReadPin(dir)
	if err != nil {
		return err
	}
	if ok {
		fmt.Println("parent_pin_set")
		return nil
	}
	fmt.Println("parent_pin_unset")
	return nil
}

func runPinSet(args []string) error {
	fs := flag.NewFlagSet("pin set", flag.ContinueOnError)
	home := fs.String("home", "", "household dir (default ~/.local/share/kidtimer)")
	base := fs.String("url", os.Getenv("KIDTIMER_URL"), "optional kid daemon URL")
	token := fs.String("token", os.Getenv("KIDTIMER_TOKEN"), "optional parent bearer")
	desk := fs.String("desk", "http://127.0.0.1:8741", "parent desk URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	digits, err := readPIN(os.Stdin)
	if err != nil {
		return err
	}
	hash, err := pin.Hash(digits)
	if err != nil {
		return err
	}
	dir, err := shareDir(*home)
	if err != nil {
		return err
	}
	if err := household.WritePin(dir, hash); err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"hash": hash})
	if *base != "" && *token != "" {
		if err := dump(putJSON(strings.TrimRight(*base, "/")+"/v1/parent-pin", *token, body)); err != nil {
			return err
		}
	}
	pushDeskKids(*desk, body)
	fmt.Fprintln(os.Stderr, "parent pin set")
	return nil
}

func readPIN(in io.Reader) (string, error) {
	sc := bufio.NewScanner(in)
	if f, ok := in.(*os.File); ok {
		if fi, err := f.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
			fmt.Fprint(os.Stderr, "Parent PIN (4 digits): ")
			if !sc.Scan() {
				if err := sc.Err(); err != nil {
					return "", err
				}
				return "", fmt.Errorf("pin is required")
			}
			first := strings.TrimSpace(sc.Text())
			fmt.Fprint(os.Stderr, "Again: ")
			if !sc.Scan() {
				if err := sc.Err(); err != nil {
					return "", err
				}
				return "", fmt.Errorf("pin confirm is required")
			}
			second := strings.TrimSpace(sc.Text())
			if first != second {
				return "", fmt.Errorf("pins did not match")
			}
			if !pin.ValidDigits(first) {
				return "", fmt.Errorf("pin must be 4 digits")
			}
			return first, nil
		}
	}
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("pin is required")
	}
	digits := strings.TrimSpace(sc.Text())
	if !pin.ValidDigits(digits) {
		return "", fmt.Errorf("pin must be 4 digits")
	}
	return digits, nil
}

func putJSON(url, token string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func pushDeskKids(desk string, body []byte) {
	if desk == "" {
		return
	}
	resp, err := http.Get(strings.TrimRight(desk, "/") + "/v1/household")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}
	var hh reverse.Household
	if err := json.NewDecoder(resp.Body).Decode(&hh); err != nil {
		return
	}
	for _, kid := range hh.Kids {
		if kid.ID == "" {
			continue
		}
		url := strings.TrimRight(desk, "/") + "/v1/kids/" + string(kid.ID) + "/parent-pin"
		req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
