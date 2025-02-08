# swissknife

## usage

write a `commands.yaml`

```yaml
commands:
  - name: "Disk Usage"
    command: "df -kh"
    repeat: 5
  - name: "Current Date"
    command: "date"
    repeat: 1 
  - name: "Current User"
    command: "whoami"
    repeat: 0
  - name: "Current User"
    command: "whoami"
    repeat: 0
  - name: "Memory Usage"
    command: "netstat -an | grep ESTABLISHED"
    repeat: 2
  - name: "Memory Usage"
    command: "netstat -an | grep ESTABLISHED"
    repeat: 2
```

run with:

```shell
$> ./swissknife -cfg=./commands.yaml
```

## ui

![screenshot](./screenshot.png)
