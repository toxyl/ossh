# oSSH

... is a dirty mix of honey and tar, delivered by a fake SSH server. 

Once running it will patiently wait for bots going after that sweet honey. When a bot tries to connect for the first time oSSH will check if username and password are already recorded. In that case it will kick the bot and wait for it to come back. If the bot has something new (either username or password), oSSH will gladly let the bot in and record the credentials. For bots that offer a username and a password that oSSH doesn't know, oSSH will roll dice to decide whether to let the bot in. This applies to new hosts, known hosts will be let it most of the time. Some bots prefer to hand over public keys, we gladly record those, too. Unless we already have that key, that's a good reason to roll dice again.

Once inside, oSSH will add some tar to the mix. The bot can run commands and access a filesystem, but it will be painfully slow (unless configured differently) and all data returned will be fake. Meanwhile oSSH will record what the bot is doing, fingerprint it and store it in an ASCIICast file for manual inspection.

In addition to being painfully slow, the bot will connect to a pretty broken system where many commands result in errors reminiscent of failing hardware, bad configuration and alike. 

How oSSH behaves can be configured via a YAML config file, a fake file system and command templates. 

oSSH can also sync with other oSSH nodes to share hosts, user names, passwords and payloads. 

## Features
- Low memory and CPU footprint (runs perfectly fine on a $5 DigitalOcean droplet)
- [Ansible Playbook](#ansible-playbook) to make deployment/update of a cluster easy
- [Data Collection](#data-collection) for analysis, blacklisting, and so on
- [Fake SSH Server](#fake-ssh-server) with support for:
  - [Password auth](#password-auth)
  - [Public key auth](#public-key-auth)
  - [SCP file uploads](#scp-support)
  - [Randomized wait times](#randomized-wait-times) 
  - [Rate limited I/O](#rate-limited-io) 
  - [IP whitelist](#ip-whitelist)
- [Fake File System](#fake-file-system-ffs) (FFS) using an OverlayFS
- [Fake Shell](#fake-shell) command processing in multiple categories:
  - Regular expression [rewriters](#rewriters-config) to transform input before processing 
  - [Simple](#simple-config) (exact string match to response, via [config](#configuration))
  - [OS error respones](#os-error-responses) (command match to error, via [config](#configuration)):
    - [Permission denied](#permission_denied-config)
    - [Disk error](#disk_error-config)
    - [Commmand not found](#command_not_found-config)
    - [File not found](#file_not_found-config)
    - [Not implemented](#not_implemented-config)
  - [Templates](#command-templates) (more sosphisticated responses using Golang templates)
  - [Built-in commands](#built-in-commands) that mimic the behavior of real commands like `cd`, `ls`, `rm`, ...
- [Sync Server](#sync-server) to distribute user names, host IPs, passwords and payloads between cluster nodes
  - [IP whitelist](#ip-whitelist-1)
- [Dashboard](#dashboard) with:
  - [Node & cluster stats](#node--cluster-stats)
  - [Console](#console-viewer)
  - [Config editor](#config-editor) 
  - [Payload viewer](#payloads-viewer) via HTTPS server 
  - [IP whitelist](#ip-whitelist-2)

## Installation
It is strongly recommended that you install oSSH on a machine that is only used for that purpose to minimize the impact should an attacker manage to break out of oSSH. DigitalOcean's $5 droplets, for example, work perfectly fine for this task.

### Ansible Playbook
If you have Ansible installed, this is the route to take.  
If you don't have Ansible yet, this is a good moment to install it: `apt install ansible`.  
You can find all further instructions [here](ansible/README.md).

### Manual
The following assumes that you will use `/etc/ossh` as [data directory](#data-directory). If you want something else you need to substitute accordingly and set `path_data` in the config.

First of all you need to become root (or run everything with `sudo`). Then get the repo and build the executable:
```bash
git clone https://github.com/Toxyl/ossh.git
cd ossh
CGO_ENABLED=0 go build
mv ossh /usr/local/bin/
```

Create directories and copy data from the repo:
```bash
mkdir -p /etc/ossh/{captures,commands,ffs}
cp -R commands/* /etc/ossh/commands/
cp -R ffs/* /etc/ossh/ffs/
cp ossh.service /etc/systemd/system/ossh.service
cp config.example.yaml /etc/ossh/config.yaml
```

Configure your instance:
```bash
nano /etc/ossh/config.yaml
```

And then enable the service:
```bash
systemctl enable ossh
```

Finally you can start trapping bots in that sweet tar:
```bash
service ossh start
```

You can monitor oSSHs operation using `journalctl`:
```bash
journalctl -u ossh -f --output cat
```

Or control it via its web interface, which will be started on `0.0.0.0:443` (default) or according to the config (`webinterface`).

## Data Directory
If you don't want to keep data in the default location (`/etc/ossh`), you can define an alternate location in the config like this:
```yaml
path_data: /usr/share/ossh
```

Within that directory you will find bind a bunch of files with data collected by oSSH:
| File | Description |
| --- | --- |
| `hosts.txt` | List of attacker IPs |
| `users.txt` | List of user names |
| `passwords.txt` | List of passwords |
| `payloads.txt` | List of payload fingerprints |

### Captures Subdirectory
The subdirectory `captures` is the collection of payloads, public SSH keys and SCP file uploads received from bots. Whenever a bot connects oSSH will record what it's doing and then save that recording as an ASCIICast v2 file which you can play back with [`asciinema`](https://asciinema.org/) or the dashboard. oSSH will attempt to categorize payloads by prefixing the SHA fingerprint of the payload with locality sensitive hash. This approach is far from perfect (PRs for better solutions are welcome!), but it does work better than pure SHA fingerprints.  
Only the payloads from the `captures` directory are synced with other nodes at the moment, but that might change in the future.

### Commands Subdirectory
The subdirectory `commands` contains templates for commands that need more elaborate behavior. These commands are baked into the executable and extracted when the executable is run, existing files will **NOT** be overwritten. These [Golang templates](https://pkg.go.dev/text/template) can be modified at runtime.

### Fake File System Subdirectory
The subdirectory `ffs` contains data of the [Fake File System](#fake-file-system-ffs).

## Data Collection
### Host IPs
All IPs connecting to the [Fake SSH Server](#fake-ssh-server) will be collected in the file `hosts.txt` in the installation directory. When running a cluster these will be regularly synced with the other nodes.  
Whitelisted IPs are excluded from data collection.

### User Names
All user names used to connect to the [Fake SSH Server](#fake-ssh-server) will be collected in the file `users.txt` in the installation directory. When running a cluster these will be regularly synced with the other nodes.  
User names used by whitelisted IPs are excluded from data collection.

### Passwords
All passwords used to connect to the [Fake SSH Server](#fake-ssh-server) will be collected in the file `passwords.txt` in the installation directory. When running a cluster these will be regularly synced with the other nodes.  
Passwords used by whitelisted IPs are excluded from data collection.

### Public SSH Keys
All public SSH keys used to connect to the [Fake SSH Server](#fake-ssh-server) will be collected in the directory `captures/ssh-keys` in the installation directory. These are currently not synced with the other nodes.  
Keys used by whitelisted IPs are excluded from data collection.

### Payloads
Everything run after logging into the [Fake SSH Server](#fake-ssh-server) will be recorded and collected in the directory `captures` in the installation directory.  
When an SSH session ends all of its input will be compared to already recorded payloads. Existing payloads will not be overwritten. New payloads will be stored and then send to all known nodes.  
The file name used to store a payload contains a locality sensitive hash followed by a SHA1 hash in an attempt to group similar payloads.  
Sessions by whitelisted IPs are excluded from data collection.

### SCP Uploads
All files uploaded to the [Fake SSH Server](#fake-ssh-server) will be collected in the directory `captures/scp-uploads` in the installation directory. These are currently not synced with the other nodes.  
Be aware that SCP file uploads by whitelisted IPs will **not** be excluded from data collection.

## Fake SSH Server
### Password Auth
When a bot tries to connect for the first time oSSH will check if username and password are already recorded. In that case it will kick the bot and wait for it to come back. If the bot has something new (either username or password), oSSH will gladly let the bot in and record the credentials. For bots that offer a username and a password that oSSH doesn't know, oSSH will let it in if the current second is divisable by 3. This applies to new hosts, known hosts will be let it most of the time, unless the current second is divisable by 7. 

### Public Key Auth
Some bots prefer to hand over public keys rather than passwords, but we gladly record those, too. Unless we already have the given key, that's a good reason to roll dice - whenever the current second is divisable by 3 the bot will be rejected.  

### Rate Limited I/O
oSSH slows down responses to simulate a slow machine and to waste the bots time. This rate limit can be defined in the config (`ratelimit`). Sometimes bots run commands with little output, so oSSH will add some penalty for every input character to slow things down a bit more for them. This can be defined in the config as well (`input_delay`).  
You can also use these config variables to configure a fast oSSH instance that is intended for data collection instead of trapping bots in tar. You could, for example, use a 2:7 ratio of fast nodes in your cluster, so most slow down bots, while some collect data (such as user names, passwords, etc.) and share it with the cluster.

Sync operations between nodes run via their own TCP connections and are exempt from these restrictions.

### Randomized Wait Times
In theory a bot could identify an oSSH instance by measuring the timing of responses. To prevent this kind of fingerprinting, oSSH will insert randomized wait times during different stages of the SSH connection. 

### SCP Support
oSSH supports file uploads using SCP. Downloading files via SCP is not supported yet.  
The uploaded files will be stored in the OverlayFS of the uploader's session and, if we don't have them yet, in the `scp-uploads` directory within the [captures directory](#captures-directory) of oSSH. *Unlike other data this is not synced between nodes at the moment.* 

### IP Whitelist
Whitelisted IPs are excluded from most rate limiting and data (such as user names, passwords, public keys) will not be collected. 

## Fake File System (FFS) 
### Default FS
The subdirectory `ffs/defaultfs` contains the files and directories bots can browse. The FFS is baked into the executable and extracted when the executable is run, existing files will **NOT** be overwritten. You can modify the extracted contents at runtime to react to new payloads. For example: if bots commonly `cat` a specific file, you can create a very lengthy fake version of that file in the `ffs/defaultfs` directory of the oSSH instance. Next time a bot `cat`s it, it will be waiting for a long time :D 

### Sandboxes
The subdirectory `ffs/sandboxes` contains OverlayFS sandboxes per host IP. This is where a bot's file system changes end up. 

## Fake Shell
Once a bot connects to oSSH and requests a shell, it will interact with the Fake Shell. That parses the bots input into instructions and tries to evaluate them. To do so it extracts the command and executes a series of steps to generate a response. The `commands` section of the config allows you to customize oSSHs responses to commands. They are evaluated in the following order:

### `rewriters` (config)
These are pairs of regular expressions and replacements which will be executed in the given order on any user/bot input. Be aware that recordings are made after rewriters have been applied, i.e. your recorded payloads may not reflect the payload as given by the bot.

### `exit` (config)
If a command matches this list the connection will be terminated with a time-wasting response that consists of a repeated sequence of a space followed by a backspace which makes it look empty but potentially take a long time to process. How often that sequence is repeated is random, at least one will be send, at most one thousand.

### `simple` (config)
These are pairs with a command to match and a response. Responses can use some template variables so one can, e.g., simulate the `whoami` command. Available variables are:  
| Variable | Effect |
| --- | --- |
| `{{ .User }}` | User name the attacker logged in with |
| `{{ .IP }}` | IP of the attacker |
| `{{ .IPLocal }}` | IP of oSSH |
| `{{ .Port }}` | Port of the attacker |
| `{{ .PortLocal }}` | Port of oSSH |
| `{{ .HostName }}` | Host name of oSSH |
| `{{ .InputRaw }}` | Raw input line that matched the command |
| `{{ .Command }}` | Command that matched |
| `{{ .Arguments }}` | Array with the arguments |

### OS Error Responses 
#### `permission_denied` (config)
If a command matches this list oSSH will respond with `{{ .Command }}: permission denied`.

#### `disk_error` (config)
If a command matches this list oSSH will respond with a string of [random garbage characters](#bullshit-config), followed by `end_request: I/O error`.

#### `command_not_found` (config)
If a command matches this list oSSH will respond with `{{ .Command }}: command not found`.

#### `file_not_found` (config)
If a command matches this list oSSH will respond with `"{{ .Command }}": No such file or directory (os error 2)`.

#### `not_implemented` (config)
If a command matches this list oSSH will respond with `{{ .Command }}: Function not implemented`.

### `bullshit` (config)
If a command matches this list oSSH will respond with random garbage bytes. Somewhere between 1 and 1000 of them. These can (and probably will) include non-printable characters.  

### Command Templates
If none of the above steps matched, oSSH will look for a response template matching the command and parse that. 
You can also create your own response templates using Golang templating, but it does require that you build oSSH yourself or use the [Ansible playbook](#ansible-playbook) because all templates are baked into the executable. You can, however, add new templates to the [commands directory](#commands-directory) of your instance and restart it. But, until you remove the files, these will be used, even if oSSH ships a newer version!

### Built-in Commands
If there is no matching command template oSSH will check if there is a built-in command to handle the input and if so, generate the response using that command.  

### Undefined
If there is still no match oSSH will simply return `{{ .Command }}: command not found`.

## Sync Server
oSSH nodes can only sync if both nodes added the other to their config. Each instance will report its stats/data to all nodes it is allowed to sync with. As a consequence each node only knows of itself and its defined neighbors. Data sync between nodes is handled by a custom TCP sync server. Assuming you have nodes running on `192.168.0.10`, `192.168.0.20` and `192.168.0.30`, the config could look like this (remember to restart the oSSH nodes after adjusting the config):

### Node 1 (`192.168.0.10`)
```yaml
sync:
  interval: 10 # in minutes
  nodes:
    - host: 192.168.0.20
      port: 1337
    - host: 192.168.0.30
      port: 1337
```

### Node 2 (`192.168.0.20`)
```yaml
sync:
  interval: 10 # in minutes
  nodes:
    - host: 192.168.0.10
      port: 1337
    - host: 192.168.0.30
      port: 1337
```

### Node 3 (`192.168.0.30`)
```yaml
sync:
  interval: 10 # in minutes
  nodes:
    - host: 192.168.0.10
      port: 1337
    - host: 192.168.0.20
      port: 1337
```

### IP Whitelist
Whitelisted IPs are allowed to communicate with the sync server. All other IPs will receive [bullshit data](#bullshit-config).

## Dashboard
oSSH comes with a dashboard that allows you to watch and filter the console output, check node & cluster stats, edit the config or view recorded payloads.

### Node & Cluster Stats
At the bottom of the dashboard you can find a black bar with stats for this node (top line) and this node + neighbors (bottom line).

### Console Viewer
Shows the logs of the oSSH instance. The output can be filtered by subsystem and message type. Single click on a filter selects it and deselects all others of the same type. Use CTRL+Click to toggle filters.

### Config Editor
You can edit the entire config via the dashboard. The reload functionality provided by it, however, will not restart the [Fake SSH Server](#fake-ssh-server) nor the sync server. It is most useful to update command responses on the fly with having to restart the oSSH service.

### Payloads Viewer
Here you can review the latest payloads. The overview is sortest newest first. Select a payload to view the recording (input & output) of it. Sometimes payloads can get damaged (e.g. transfer error, out of disk space), the player then shows a blinking cursor in the upper right.

### IP Whitelist
Whitelisted IPs are allowed to access the dashboard. HTTP requests from all other IPs will be redirected to themselves.
