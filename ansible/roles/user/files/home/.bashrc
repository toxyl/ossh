case $- in
    *i*) ;;
      *) return;;
esac

HISTCONTROL=ignoreboth

shopt -s histappend

HISTSIZE=1000
HISTFILESIZE=2000

shopt -s checkwinsize

if [ -z "${debian_chroot:-}" ] && [ -r /etc/debian_chroot ]; then
    debian_chroot=$(cat /etc/debian_chroot)
fi

case "$TERM" in
    xterm-color) color_prompt=yes;;
esac

force_color_prompt=yes

if [ -n "$force_color_prompt" ]; then
    if [ -x /usr/bin/tput ] && tput setaf 1 >&/dev/null; then
        color_prompt=yes
    else
        color_prompt=
    fi
fi

if [ "$color_prompt" = yes ]; then
    C1=$(shuf -i 31-37 -n 1 | tr -d "\n")
    C2="$C1"
    while [ "$C2" == "$C1" ] ; do
            C2=$(shuf -i 31-37 -n 1 | tr -d "\n")
    done
    
    C3="$C2"
    while [ "$C3" == "$C2" ] || [ "$C3" == "$C1" ] ; do
            C3=$(shuf -i 31-37 -n 1 | tr -d "\n")
    done

    HOST=$(hostname -I | awk '{print $1}' | tr -d "\n")
    HOSTNAME=$(hostname | tr -d "\n")
    PS1='\[\033[01;${C2}m\]$HOST\[\033[00m\] ⏵ \[\033[01;${C1}m\][$HOSTNAME]\[\033[00m\] ⏵ ${debian_chroot:+($debian_chroot)}\[\033[01;${C2}m\]${PUSER_START}\u${PUSER_END}\[\033[00m\] ⏵ \[\033[01;${C3}m\]\w\[\033[00m\]\n→ '
else
    PS1='$HOSTNAME ⏵ ${debian_chroot:+($debian_chroot)}\u@\h:\w\$ '
fi

unset color_prompt force_color_prompt

# If this is an xterm set the title to user@host:dir
case "$TERM" in
xterm*|rxvt*)
    PS1="\[\e]0;${debian_chroot:+($debian_chroot)}\u@\h: \w\a\]$PS1"
    ;;
*)
    ;;
esac

# enable color support of ls and also add handy aliases
if [ -x /usr/bin/dircolors ]; then
    test -r ~/.dircolors && eval "$(dircolors -b ~/.dircolors)" || eval "$(dircolors -b)"
    alias ls='ls --color=auto'
    alias dir='dir --color=auto'
    alias vdir='vdir --color=auto'
fi

if [ -f ~/.bash_aliases ]; then
    . ~/.bash_aliases
fi

# enable programmable completion features (you don't need to enable
# this, if it's already enabled in /etc/bash.bashrc and /etc/profile
# sources /etc/bash.bashrc).
if ! shopt -oq posix; then
  if [ -f /usr/share/bash-completion/bash_completion ]; then
    . /usr/share/bash-completion/bash_completion
  elif [ -f /etc/bash_completion ]; then
    . /etc/bash_completion
  fi
fi
