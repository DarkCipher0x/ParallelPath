package seal

import "os"

func devPlain() bool { return os.Getenv("PP_DEV_PLAIN_KEY") == "1" }
