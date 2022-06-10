package main

import (
	"errors"
	"net"
	"strings"
)

type SyncCommand func(ssc *SyncServerConnection, args []string) (string, error)

var SyncCommands = map[string]SyncCommand{
	"NAME": func(ssc *SyncServerConnection, args []string) (string, error) {
		return Conf.HostName, nil
	},
	"SYNC": func(ssc *SyncServerConnection, args []string) (string, error) {
		if len(args) < 1 {
			return "", errors.New("need your fingerprint")
		}
		fp := args[0]
		if fp == "" {
			return "", errors.New("need your fingerprint")
		}
		host, _, _ := net.SplitHostPort(ssc.conn.RemoteAddr().String())
		fpsrv := SrvOSSH.Loot.Fingerprint()
		if fp == fpsrv {
			LogSyncCommands.Debug("Ignored SYNC request from %s: %s (we have: %s)", colorHost(host), colorHighlight(fp), colorHighlight(fpsrv))
			return "", nil
		}
		fpRemote := strings.Split(fp, ":")
		fpLocal := strings.Split(fpsrv, ":")

		if len(fpRemote) != len(fpLocal) {
			LogSyncCommands.Debug("Ignored SYNC request from %s, fingerprints are not the same length: %s (we have: %s)", colorHost(host), colorHighlight(fp), colorHighlight(fpsrv))
			return "", nil
		}

		if len(fpRemote) == 0 || len(fpLocal) == 0 {
			LogSyncCommands.Debug("Ignored SYNC request from %s, one of the fingerprints is empty: %s (we have: %s)", colorHost(host), colorHighlight(fp), colorHighlight(fpsrv))
			return "", nil
		}

		syncList := []string{}
		if fpRemote[0] != fpLocal[0] {
			syncList = append(syncList, "hosts")
		}

		if fpRemote[1] != fpLocal[1] {
			syncList = append(syncList, "users")
		}

		if fpRemote[2] != fpLocal[2] {
			syncList = append(syncList, "passwords")
		}

		if fpRemote[3] != fpLocal[3] {
			syncList = append(syncList, "payloads")
		}

		sl := strings.Join(syncList, ",")
		return sl, nil
	},
	"HOSTS": func(ssc *SyncServerConnection, args []string) (string, error) {
		return ImplodeLines(SrvOSSH.Loot.GetHosts()), nil
	},
	"USERS": func(ssc *SyncServerConnection, args []string) (string, error) {
		return ImplodeLines(SrvOSSH.Loot.GetUsers()), nil
	},
	"PASSWORDS": func(ssc *SyncServerConnection, args []string) (string, error) {
		return ImplodeLines(SrvOSSH.Loot.GetPasswords()), nil
	},
	"PAYLOADS": func(ssc *SyncServerConnection, args []string) (string, error) {
		return ImplodeLines(SrvOSSH.Loot.GetPayloads()), nil
	},
	"ADD-HOST": func(ssc *SyncServerConnection, args []string) (string, error) {
		if len(args) < 1 {
			return "", nil
		}
		added := SrvOSSH.Loot.AddHosts(args)
		if added > 0 {
			host, _, _ := net.SplitHostPort(ssc.conn.RemoteAddr().String())
			LogSyncCommands.OK("%s donated %s host(s)", colorHost(host), colorInt(added))
			SrvOSSH.SaveData()
		}
		return "", nil
	},
	"ADD-USER": func(ssc *SyncServerConnection, args []string) (string, error) {
		if len(args) < 1 {
			return "", nil
		}
		added := SrvOSSH.Loot.AddUsers(args)
		if added > 0 {
			host, _, _ := net.SplitHostPort(ssc.conn.RemoteAddr().String())
			LogSyncCommands.OK("%s donated %s user(s)", colorHost(host), colorInt(added))
			SrvOSSH.SaveData()
		}
		return "", nil
	},
	"ADD-PASSWORD": func(ssc *SyncServerConnection, args []string) (string, error) {
		if len(args) < 1 {
			return "", nil
		}
		added := SrvOSSH.Loot.AddPasswords(args)
		if added > 0 {
			host, _, _ := net.SplitHostPort(ssc.conn.RemoteAddr().String())
			LogSyncCommands.OK("%s donated %s password(s)", colorHost(host), colorInt(added))
			SrvOSSH.SaveData()
		}
		return "", nil
	},
	"ADD-PAYLOAD": func(ssc *SyncServerConnection, args []string) (string, error) {
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
				host, _, _ := net.SplitHostPort(ssc.conn.RemoteAddr().String())
				LogSyncCommands.OK("%s donated payload %s", colorHost(host), colorFile(pl.file))
				SrvOSSH.SaveData()
			}
		}

		return "", nil
	},
	"ADD-STATS": func(ssc *SyncServerConnection, args []string) (string, error) {
		if len(args) < 1 {
			return "", nil
		}
		host, _, _ := net.SplitHostPort(ssc.conn.RemoteAddr().String())
		stats := SrvOSSH.JSONToStats(strings.Join(args, " "))
		SrvSync.nodes.AddStats(host, stats)
		LogSyncCommands.Debug("%s reported stats", colorHost(host))
		return "", nil
	},
}
