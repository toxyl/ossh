package main

import (
	"errors"
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
	"PAYLOADS": func(args []string) (string, error) {
		return ImplodeLines(SrvOSSH.Loot.GetPayloads()), nil
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
	"ADD-PAYLOAD": func(args []string) (string, error) {
		if len(args) < 2 {
			return "", errors.New("need fingerprint and data")
		}

		hash := args[0]
		data := args[1]
		pl := NewPayload()
		pl.SetHash(hash)
		pl.DecodeFromString(data)
		pl.Save()
		if pl.Exists() {
			if SrvOSSH.Loot.AddPayload(hash) {
				LogSyncCommands.OK("Added payload %s", colorFile(pl.file))
				SrvOSSH.SaveData()
			}
		}

		return "", nil
	},
}
