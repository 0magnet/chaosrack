// Copyright 2025 The TCell Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use file except in compliance with the License.
// You may obtain a copy of the license at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// unicode just displays a Unicode test on your screen.
// Press ESC to exit the program.
package unicode

import (
	"github.com/gdamore/tcell/v3"
)

var row = 0
var style = tcell.StyleDefault

func putln(s tcell.Screen, str string) {

	s.PutStrStyled(1, row, str, style)
	row++
}
