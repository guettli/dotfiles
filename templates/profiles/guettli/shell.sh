# Profile "guettli": Thomas Güttler's personal settings.

pathadd() {
    case ":$PATH:" in
    *":$1:"*) ;;
    *) PATH="$1:$PATH" ;;
    esac
}
pathadd "$HOME/projects/git-tips/scripts"
pathadd "$HOME/scripts"
unset -f pathadd
export PATH

export K9S_FEATURE_GATE_NODE_SHELL=true
if [ "$TERM_PROGRAM" != "vscode" ]; then
    export EDITOR="vim"
fi
