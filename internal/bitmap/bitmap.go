package bitmap

type Bitmap struct {
	data []uint64
	size int
}

func BitMap(size int) *Bitmap {
	if size <= 0 {
		size = 1
	}

	length := (size + 63) / 64
	return &Bitmap{
		data: make([]uint64, length),
		size: size,
	}
}

func (b *Bitmap) Set(pos int) {
	if pos < 0 || pos >= b.size {
		return
	}

	idx := pos / 64
	shift := pos % 64
	b.data[idx] |= 1 << shift
}

func(b *Bitmap) Clear(pos int) {
	if pos < 0 || pos >= b.size {
		return
	}

	idx := pos / 64
	shift := pos % 64
	b.data[idx] &^= 1 << shift
}

func (b *Bitmap) IsSet(pos int) bool {
	if pos < 0 || pos >= b.size {
		return false
	}

	idx := pos / 64
	shift := pos % 64
	return (b.data[idx] & (1 << shift)) != 0
}

func (b *Bitmap) FindNextClearBit(startPos int) int {
	if b.size == 0 {
		return -1
	}

	pos := startPos
	for scanned := 0; scanned < b.size; scanned++ {
		if !b.IsSet(pos) {
			return pos
		}
		pos = (pos + 1) % b.size
	}

	return -1
}