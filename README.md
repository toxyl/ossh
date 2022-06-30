# oSSH

... is a dirty mix of honey and tar, delivered by a fake SSH server. 

Once running it will patiently wait for bots going after that sweet honey. When a bot tries to connect for the first time oSSH will check if username and password are already recorded. In that case it will kick the bot and wait for it to come back. If the bot has something new (either username or password), oSSH will gladly let the bot in and record the credentials. For bots that offer a username and a password that oSSH doesn't know, oSSH will roll dice to decide whether to let the bot in. This applies to new hosts, known hosts will be let it most of the time. Some bots prefer to hand over public keys, we gladly record those, too. Unless we already have that key, that's a good reason to roll dice again.

Once inside, oSSH will add some tar to the mix. The bot can run commands and access a filesystem, but it will be painfully slow (unless configured differently) and all data returned will be fake. Meanwhile oSSH will record what the bot is doing, fingerprint it and store it in an ASCIICast file for manual inspection.

In addition to being painfully slow, the bot will connect to a pretty broken system where many commands result in errors reminiscent of failing hardware, bad configuration and alike. 

How oSSH behaves can be configured via a YAML config file, a fake file system and command templates. 

oSSH can also sync with other oSSH nodes to share hosts, user names, passwords and payloads. 

## Features
- Fake SSH server with support for [password auth](#password-auth) and [public key auth](#public-key-auth) 
- FFS ([Fake File System](#fake-file-system-ffs)) using an OverlayFS
- Fake commands in multiple categories:
  - [Simple](#simple-config) (exact string match to response)
  - [Templates](#command-templates) (more sosphisticated responses using Golang templates)
  - [Golang implemented commands](#golang-implemented-commands) that mimic the behavior of real commands like `cd`, `ls`, `rm`, ...
- Support for [SCP uploads](#scp-support)
- Configurable [sluggishness](#sluggishness) 
- Configurable [command responses](#command-responses)
- [Ansible playbook](#ansible) to make deployment/update of a cluster easy
- [Data sync](#syncing) between cluster nodes (user names, host IPs, passwords and payloads) using custom TCP server
- [Randomized wait times](#randomized-wait-times) 
- Low memory and CPU footprint (runs perfectly fine on a $5 DigitalOcean droplet)
- Dashboard with node & cluster stats, console, config editor and payload viewer via HTTPS server 
- IP whitelisting (automatically setup when using the [Ansible playbook](#ansible))
  - Whitelisted IPs are excluded from most rate limiting and data (such as user names, passwords, public keys) will not be collected, access to dashboard and sync server is allowed
  - Not whitelisted IPs will receive [bullshit data](#bullshit-config) (sync server) or will be redirected to themselves (dashboard)
- Regular expression [rewriters](#rewriters-config) to transform input before processing 
- OS error respones ([permission denied](#permission_denied-config), [disk error](#disk_error-config), [commmand not found](#command_not_found-config), [file not found](#file_not_found-config), [not implemented](#not_implemented-config)) to [configurable commands](#command-responses)
- Payload grouping using locality sensitive hashing (not great, but better than pure SHA hashing)
- 

## Installation
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

## Configuration
### Sluggishness
oSSH slows down responses to simulate a slow machine and to waste the bots time. This ratelimit can be defined in the config (`ratelimit`). Sometimes bots run commands with little output, so oSSH will add some penalty for every input character to slow things down a bit more for them. This can be defined in the config as well (`input_delay`). Obviously you can also use these config variables to configure a fast oSSH instance that is intended for data collection instead of trapping bots in tar. I, for example, use a 2:7 ratio of fast nodes in my cluster, so most slow down bots, while some collect data (such as user names, passwords, etc.) and share it with the cluster.

Sync operations between nodes run via their own TCP connections and are exempt from these restrictions.

### Command Responses
The `commands` section of the config allows you to customize oSSHs responses to commands. They are evaluated in the following order:

#### `rewriters` (config)
These are pairs of regular expressions and replacements which will be executed in the given order on any user/bot input. Be aware that recordings are made after rewriters have been applied, i.e. your recorded payloads may not reflect the payload as given by the bot.

#### `exit` (config)
If a command matches this list the connection will be terminated with a time-wasting response that consists of a repeated sequence of a space followed by a backspace which makes it look empty but potentially take a long time to process. How often that sequence is repeated is random, at least one will be send, at most one thousand.

#### `simple` (config)
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

#### `bullshit` (config)
If a command matches this list oSSH will respond with random garbage bytes. Somewhere between 1 and 1000 of them. These can (and probably will) include non-printable characters.  

#### Command templates
If none of the above steps matched, oSSH will look for a response template matching the command and parse that. 
You can also create your own response templates using Golang templating, but it does require that you build oSSH yourself or use the [Ansible playbook](#ansible) because all templates are baked into the executable. You can, however, add new templates to the [commands directory](#commands-directory) of your instace and restart it. But, until you remove the files, these will be used, even if oSSH ships a newer version!

#### Golang-implemented commands
If there is no matching command template oSSH will check if there is a Golang-implemented command to handle the input and if so, generate the response using that command.  

#### Nothing defined anywhere
If there is still no match oSSH will simply return `{{ .Command }}: command not found`.

## Syncing
If you run multiple instances of oSSH, you might want them to share their knowledge. To do so you can create credentials, store them in the config of each instance and then restart the instances. Once done they will regularly sync up with all nodes defined in their config. Assuming you have nodes running on `192.168.0.10`, `192.168.0.20` and `192.168.0.30`, the config could look like this:

### Node 1 (`192.168.0.10`)
```yaml
sync:
  interval: 1 # in minutes
  nodes:
    - host: 192.168.0.20
      port: 22
      user: 078e5067ec45f123a11b0845b5ddba3fea63e243118454ce07f85d7639eb4ec4
      password: 91ca82fc115605a4e21de7f9fc005b450ef6baa69fef56dbfbaf64375c21fd4f
    - host: 192.168.0.30
      port: 22
      user: 8fe36196f2c4bb5a63f390c3e2e1152a3ababcd3f41c1295b86f773c9c53c632
      password: ea5f6f80595c72c5e1ee8198a651f7584a3b293afbefcf228dd8b1659b6864c9
```

### Node 2 (`192.168.0.20`)
```yaml
sync:
  interval: 1 # in minutes
  nodes:
    - host: 192.168.0.10
      port: 22
      user: 42ba9f2b9b6e44a1b2744a243201d3147d174232de467899ab7e20df374101df
      password: 8dd88f838d80197836c59ccdd8fbde1feed27688a443132be5c0cb0e999b603f
    - host: 192.168.0.30
      port: 22
      user: 8fe36196f2c4bb5a63f390c3e2e1152a3ababcd3f41c1295b86f773c9c53c632
      password: ea5f6f80595c72c5e1ee8198a651f7584a3b293afbefcf228dd8b1659b6864c9
```

### Node 3 (`192.168.0.30`)
```yaml
sync:
  interval: 1 # in minutes
  nodes:
    - host: 192.168.0.10
      port: 22
      user: 42ba9f2b9b6e44a1b2744a243201d3147d174232de467899ab7e20df374101df
      password: 8dd88f838d80197836c59ccdd8fbde1feed27688a443132be5c0cb0e999b603f
    - host: 192.168.0.20
      port: 22
      user: 078e5067ec45f123a11b0845b5ddba3fea63e243118454ce07f85d7639eb4ec4
      password: 91ca82fc115605a4e21de7f9fc005b450ef6baa69fef56dbfbaf64375c21fd4f
```

## Data directory
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

### Captures directory
The subdirectory `captures` is the collection of payloads received from bots. Whenever a bot connects oSSH will record what it's doing and then save that recording as an ASCIICast v2 (you can use [`asciinema`](https://asciinema.org/) to play them back). Captures are saved per host, so you can, e.g., identify especially aggressive bots. The last part of the file name is the fingerprint of the sequence. Existing files will not be overwritten. 

### Fake File System (FFS) 
The subdirectory `ffs` contains the files and directories bots can browse. You can modify the directory content at runtime to react to new payloads. For example: if bots commonly `cat` a specific file, you can create a very lengthy fake version of that file in the `ffs` directory. Next time a bot `cat`s it, it will be waiting for a long time :D 

### Commands directory
The subdirectory `commands` contains templates for commands that need more elaborate behavior. Like the `ffs` directory it can be modified at runtime. These files are Golang templates, see [this](https://pkg.go.dev/text/template) for more information in regards to the templating language.

## Fake SSH Server
### SCP support
oSSH supports file uploads using SCP. Downloading files via SCP is not supported yet.  
The uploaded files will be stored in the OverlayFS of the uploader's session and, if we don't have them yet, in the `scp-uploads` directory within the [captures directory](#captures-directory) of oSSH. *Unlike other data this is not synced between nodes at the moment.* 

### Password auth
When a bot tries to connect for the first time oSSH will check if username and password are already recorded. In that case it will kick the bot and wait for it to come back. If the bot has something new (either username or password), oSSH will gladly let the bot in and record the credentials. For bots that offer a username and a password that oSSH doesn't know, oSSH will let it in if the current second is divisable by 3. This applies to new hosts, known hosts will be let it most of the time, unless the current second is divisable by 7. 

### Public key auth
Some bots prefer to hand over public keys rather than passwords, but we gladly record those, too. Unless we already have the given key, that's a good reason to roll dice - whenever the current second is divisable by 3 the bot will be rejected.  

### Randomized wait times
In theory a bot could identify an oSSH instance by measuring the timing of responses. To prevent this kind of fingerprinting, oSSH will insert randomized wait times during different stages of the SSH connection. 