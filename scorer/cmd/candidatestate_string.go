// Code generated by "stringer -type=candidateState"; DO NOT EDIT.

package cmd

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[candidateUnknown-0]
	_ = x[candidateIn-1]
	_ = x[candidateOut-2]
	_ = x[candidateBlock-3]
}

const _candidateState_name = "candidateUnknowncandidateIncandidateOutcandidateBlock"

var _candidateState_index = [...]uint8{0, 16, 27, 39, 53}

func (i candidateState) String() string {
	if i >= candidateState(len(_candidateState_index)-1) {
		return "candidateState(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _candidateState_name[_candidateState_index[i]:_candidateState_index[i+1]]
}