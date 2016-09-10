# megaannex-go

## About
This is a drop-in replacement for [megaannex](https://github.com/TobiasTheViking/megaannex). It should be compatible in every way except the initial environment variables

This project makes use of [go-mega](https://github.com/t3rm1n4l/go-mega) for the connection to [mega](https://mega.co.nz)

## Prerequisites
* Go
* git-annex[^1]
* go-mega

[^1]: Not required to build the project

## Installation
    go get github.com/t3rm1n4l/go-mega
    git clone https://github.com/dxtr/megaannex-go.git
    cd megaannex-go
    go build

Then rename megaannex-go to git-annex-remote-mega and copy it to your PATH

## Usage
Init a remote:

`MEGA_USERNAME="<name>" MEGA_PASSWORD="<password>" git annex initremote <name> type=external externaltype=mega encryption=hybrid keyid=<keyid> mac=HMACSHA256 folder=<folder>
`

See the git-annex manual for more options and their respective values

## Issues
Known issues:

1. Removal of files is not working yet

If you have found more issues please use the issue tracker at [github](https://github.om/dxtr/megaannex-go)

## TODO
1. Implement removal of files
2. Create a Makefile?
3. Clean up and reorganize the code
4. Optimize what can be optimized

## License
This program is released under the MIT license (See the LICENSE file)
