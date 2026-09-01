package playlist

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/robertkrimen/otto"
)

//go:embed qqmusic_encrypt.js
var qqmusicJS string

var (
	signVM   *otto.Otto
	signMu   sync.Mutex
	signInit sync.Once
	initErr  error
)

func getQQMusicSign(data string) (string, error) {
	signInit.Do(func() {
		vm := otto.New()
		if _, err := vm.Run(qqmusicJS); err != nil {
			initErr = fmt.Errorf("init qqmusic js engine failed: %w", err)
			return
		}
		signVM = vm
	})

	if initErr != nil {
		return "", initErr
	}

	signMu.Lock()
	defer signMu.Unlock()

	val, err := signVM.Call("get_sign", nil, data)
	if err != nil {
		return "", fmt.Errorf("call get_sign js function failed: %w", err)
	}
	return val.String(), nil
}
