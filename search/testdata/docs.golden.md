**Files:**

- `animals/cute cat.jpeg`
- `animals/cat jumps.mp4`
- `animals/caterpillar.png`
- `animals/Cat & Dog play.mkv`
- `dogmas/catalog.zip`

**Search Requests:**

- `caterpillar` - search for filepaths that have the same prefixes as `caterpillar` (`cat`, `cate`, `cater`, ...). Results:
  - `animals/caterpillar.png`
  - `animals/Cat & Dog play.mkv`
  - `animals/cat jumps.mp4`
  - `animals/cute cat.jpeg`
  - `dogmas/catalog.zip`
- `"caterpillar"` - search for filepaths that have exactly `caterpillar`. Results:
  - `animals/caterpillar.png`
- `cat dog` - search for filepaths that have the same prefixes as both `cat` and `dog`. Results:
  - `animals/Cat & Dog play.mkv`
  - `dogmas/catalog.zip`
- `cat dog -zip` - search for filepaths that have the same prefixes as both `cat` and `dog`, but don't have exactly `zip`. Results:
  - `animals/Cat & Dog play.mkv`
- `-"dog" -png -jumps` - search for filepaths that don't have exactly `dog`, `png` and `jumps`. Results:
  - `animals/cute cat.jpeg`
- `dog "/cat" -mkv` - search for filepaths that have the same prefixes as `dog`, have exactly `/cat` and don't have exactly `mkv`. Results:
  - `dogmas/catalog.zip`
- `animals -"cat & dog"` - search for filepaths that have the same prefixes as `animals` and don't have exactly `cat & dog`. Results:
  - `animals/cat jumps.mp4`
  - `animals/caterpillar.png`
  - `animals/cute cat.jpeg`
