package main

import (
	"errors"
	"strings"
)

type SyncCommand func(args []string) (string, error)

var SyncCommands = map[string]SyncCommand{
	"NAME": func(args []string) (string, error) {
		return Conf.HostName, nil
	},
	"SYNC": func(args []string) (string, error) {
		if len(args) < 1 {
			return "", errors.New("need your fingerprint")
		}
		fp := args[0]
		fpsrv := SrvOSSH.Loot.Fingerprint()
		if fp == fpsrv {
			return "", nil
		}
		return fpsrv, nil
	},
	"STATS": func(args []string) (string, error) {
		return SrvOSSH.statsJSONSimple(), nil
	},
	"HOSTS": func(args []string) (string, error) {
		return ImplodeLines(SrvOSSH.Loot.GetHosts()), nil
	},
	"USERS": func(args []string) (string, error) {
		return ImplodeLines(SrvOSSH.Loot.GetUsers()), nil
	},
	"PASSWORDS": func(args []string) (string, error) {
		return ImplodeLines(SrvOSSH.Loot.GetPasswords()), nil
	},
	"FINGERPRINTS": func(args []string) (string, error) {
		return ImplodeLines(SrvOSSH.Loot.GetFingerprints()), nil
	},
	"ADD-HOST": func(args []string) (string, error) {
		if len(args) < 1 {
			return "", nil
		}
		added := 0
		for _, h := range args {
			if SrvOSSH.Loot.AddHost(h) {
				added++
			}
		}
		if added > 0 {
			LogSyncCommands.OK("Added %s host(s)", colorInt(added))
			SrvOSSH.SaveData()
		}
		return "", nil
	},
	"ADD-USER": func(args []string) (string, error) {
		if len(args) < 1 {
			return "", nil
		}
		added := 0
		for _, u := range args {
			if SrvOSSH.Loot.AddUser(u) {
				added++
			}
		}
		if added > 0 {
			LogSyncCommands.OK("Added %s user(s)", colorInt(added))
			SrvOSSH.SaveData()
		}
		return "", nil
	},
	"ADD-PASSWORD": func(args []string) (string, error) {
		if len(args) < 1 {
			return "", nil
		}
		added := 0
		for _, p := range args {
			if SrvOSSH.Loot.AddPassword(p) {
				added++
			}
		}
		if added > 0 {
			LogSyncCommands.OK("Added %s password(s)", colorInt(added))
			SrvOSSH.SaveData()
		}
		return "", nil
	},
	"ADD-FINGERPRINT": func(args []string) (string, error) {
		if len(args) < 1 {
			return "", nil
		}
		added := 0
		for _, f := range args {
			if SrvOSSH.Loot.AddFingerprint(f) {
				added++
			}
		}
		if added > 0 {
			LogSyncCommands.OK("Added %s fingerprint(s)", colorInt(added))
			SrvOSSH.SaveData()
		}
		return "", nil
	},
	"GET-PAYLOAD": func(args []string) (string, error) {
		if len(args) < 1 {
			return "", errors.New("need fingerprint") // TODO return payload list?
		}

		val := args[0]
		ret, err := SrvOSSH.Loot.payloads.Get(val)
		if err != nil {
			return "", nil
		}

		payload, _ := ret.Read()
		return strings.TrimSpace(payload), nil
	},
}
