// Code generated by "stringer -type=BloomFilterAlgo"; DO NOT EDIT.

package section

import "strconv"

const _BloomFilterAlgo_name = "BloomKM12BloomKM16BloomKM20BloomKM24"

var _BloomFilterAlgo_index = [...]uint8{0, 9, 18, 27, 36}

func (i BloomFilterAlgo) String() string {
	i -= 1
	if i < 0 || i >= BloomFilterAlgo(len(_BloomFilterAlgo_index)-1) {
		return "BloomFilterAlgo(" + strconv.FormatInt(int64(i+1), 10) + ")"
	}
	return _BloomFilterAlgo_name[_BloomFilterAlgo_index[i]:_BloomFilterAlgo_index[i+1]]
}
