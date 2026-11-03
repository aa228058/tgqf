package messages

import (
    "testing"
    "time"
)

func TestJoinTaskCooldownSecondsAddsBufferAfterCeiling(t *testing.T) {
    cases:=[]struct{name string;wait time.Duration;want int64}{
        {"exact seconds",2*time.Minute,121},
        {"fractional second",2*time.Minute+100*time.Millisecond,122},
        {"single second",time.Second,2},
        {"zero",0,0},
        {"negative",-time.Second,0},
    }
    for _,tc:=range cases {
        t.Run(tc.name,func(t *testing.T){
            if got:=joinTaskCooldownSeconds(tc.wait);got!=tc.want {t.Fatalf("joinTaskCooldownSeconds(%s)=%d, want %d",tc.wait,got,tc.want)}
        })
    }
}
