package main

import (
	"errors"
	"strings"
	"sync"

	"github.com/toxyl/glog"
	"github.com/toxyl/gutils"
)

type SyncCommand func(ssc *SyncServerConnection, args []string) (string, error)

type SyncCommands struct {
	commands map[string]SyncCommand
	lock     *sync.Mutex
}

func (sc *SyncCommands) Run(ssc *SyncServerConnection, command string, args []string) (string, error) {
	sc.lock.Lock()
	defer sc.lock.Unlock()
	if _, ok := sc.commands[command]; ok {
		return sc.commands[command](ssc, args)
	}
	return EmptyCommandResponse, errors.New("command not found")
}

func NewSyncCommands() *SyncCommands {
	return &SyncCommands{
		commands: map[string]SyncCommand{
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
				fpsrv := SrvOSSH.Loot.Fingerprint()
				if fp == fpsrv {
					LogSyncCommands.Debug("%s: Ignored SYNC request: %s", ssc.LogID(), glog.Highlight("already up-to-date"))
					return "", nil
				}
				fpRemote := strings.Split(fp, ":")
				fpLocal := strings.Split(fpsrv, ":")

				if len(fpRemote) != len(fpLocal) {
					LogSyncCommands.Debug("%s: Ignored SYNC request, fingerprints are not the same length: %s (we have: %s)", ssc.LogID(), glog.Highlight(fp), glog.Highlight(fpsrv))
					return "", nil
				}

				if len(fpRemote) == 0 || len(fpLocal) == 0 {
					LogSyncCommands.Debug("%s: Ignored SYNC request, one of the fingerprints is empty: %s (we have: %s)", ssc.LogID(), glog.Highlight(fp), glog.Highlight(fpsrv))
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
				return gutils.ImplodeLines(SrvOSSH.Loot.GetHosts()), nil
			},
			"USERS": func(ssc *SyncServerConnection, args []string) (string, error) {
				return gutils.ImplodeLines(SrvOSSH.Loot.GetUsers()), nil
			},
			"PASSWORDS": func(ssc *SyncServerConnection, args []string) (string, error) {
				return gutils.ImplodeLines(SrvOSSH.Loot.GetPasswords()), nil
			},
			"PAYLOADS": func(ssc *SyncServerConnection, args []string) (string, error) {
				return gutils.ImplodeLines(SrvOSSH.Loot.GetPayloads()), nil
			},
			"ADD-HOST": func(ssc *SyncServerConnection, args []string) (string, error) {
				if len(args) < 1 {
					return "", nil
				}
				added := SrvOSSH.Loot.AddHosts(args)
				if added > 0 {
					if added > 1 || Conf.Debug.SyncCommands { // to avoid log clutter
						LogSyncCommands.OK("%s: Donated %s", ssc.LogID(), glog.IntAmount(added, "host", "hosts"))
					}
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
					if added > 1 || Conf.Debug.SyncCommands { // to avoid log clutter
						LogSyncCommands.OK("%s: Donated %s", ssc.LogID(), glog.IntAmount(added, "user", "users"))
					}
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
					if added > 1 || Conf.Debug.SyncCommands { // to avoid log clutter
						LogSyncCommands.OK("%s: Donated %s", ssc.LogID(), glog.IntAmount(added, "password", "passwords"))
					}
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
						LogSyncCommands.OK("%s: Donated payload %s", ssc.LogID(), glog.File(pl.file))
						SrvOSSH.SaveData()
					}
				}

				return "", nil
			},
			"ADD-STATS": func(ssc *SyncServerConnection, args []string) (string, error) {
				if len(args) < 1 {
					return "", nil
				}
				stats := SrvOSSH.JSONToStats(strings.Join(args, " "))
				SrvSync.nodes.AddStats(ssc.Host, stats)
				LogSyncCommands.Debug("%s: Reported stats", ssc.LogID())
				return "", nil
			},
		},
		lock: &sync.Mutex{},
	}
}
