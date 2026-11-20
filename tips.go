package main

const tipsText = `What your shell can do now
==========================

History and navigation
  Ctrl-R          Search all commands you ever typed (Atuin). Type a few
                  letters, Enter runs it, Tab puts it on the command line.
  Up arrow        Previous command, as usual.
  z <part>        Jump to a directory you used before: "z proj" -> ~/projects
  Tab             Completion menu. Press Tab again to walk through it.
  **<Tab>         Fuzzy search for files: "cat **<Tab>", "cd **<Tab>"

Files and text
  ll              List files with details (also: la, ls)
  eza --tree      Show a directory as a tree (Linux only)
  bat <file>      Show a file with syntax highlighting and line numbers
  rg <text>       Search for text in all files below this directory (ripgrep)
  fd <name>       Find files by name
  micro <file>    Simple editor: Ctrl-S saves, Ctrl-Q quits, Ctrl-C/Ctrl-V work
  tldr <command>  Short examples for a command ("tldr tar")

Git
  lazygit         Full screen git: stage, commit, push with single keys
  The prompt shows the branch and whether you have changes (m = modified).
  Commits show "# Committing as: <email>", so you see which email is used.

Projects
  .envrc          direnv loads the variables of a project when you cd into it.
                  Run "direnv allow" once per project to trust its .envrc.
  mise use -g <tool>   Install an extra tool for you, e.g. "mise use -g node"
  tmux            Keep programs running after closing the terminal

Your own settings
  ~/.bashrc.local             Your bash settings (aliases, exports, ...)
  ~/.zshrc.local              Your zsh settings
  ~/.config/shell/local.sh    Settings for both bash and zsh
  ~/.gitconfig                Your git settings (they win over the managed ones)
  dotfiles never touches these files. Do not edit ~/.bashrc itself.

Keeping it up to date
  dotfiles update     Get the newest version and apply it
  dotfiles doctor     Check the setup
  dotfiles diff       Show what apply would change
`
