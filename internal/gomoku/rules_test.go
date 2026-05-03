package gomoku

import "testing"

func TestHasFive(t *testing.T) {
	var board [boardSize][boardSize]int
	for x := 3; x <= 7; x++ {
		board[6][x] = black
	}
	if !hasFive(board, 5, 6, black) {
		t.Fatal("expected horizontal five-in-a-row")
	}
}

func TestHasFiveRejectsFour(t *testing.T) {
	var board [boardSize][boardSize]int
	for y := 2; y <= 5; y++ {
		board[y][8] = white
	}
	if hasFive(board, 8, 4, white) {
		t.Fatal("did not expect four stones to win")
	}
}
