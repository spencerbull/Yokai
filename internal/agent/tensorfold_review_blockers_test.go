package agent

import (
 "context"
 "strings"
 "testing"
)

// Config.Cmd is the argv AFTER IMAGE in pinned start.sh lines 294-317.
func TestTensorFoldPinnedStartArgv(t *testing.T) {
 for _, worker := range []bool{false,true} {
  for _, variant := range []string{"actual", "missing executable", "forged executable", "forged verb", "forged model"} {
   t.Run(variant+map[bool]string{false:"-head",true:"-worker"}[worker],func(t *testing.T){
    host,rank,id := "192.168.1.191","0",strings.Repeat("1",64)
    if worker { host,rank,id="","1",strings.Repeat("2",64) }
    data:=tensorFoldInspectFixtureWithIdentity(host,rank,strings.Repeat("a",64),id)
    data=mutateTensorFoldInspectCommand(data,func(cmd []string) []string {
     switch variant {
     case "missing executable": return cmd[1:]
     case "forged executable": cmd[0]="evil"
     case "forged verb": cmd[1]="other"
     case "forged model": cmd=append(cmd,"--note",cmd[2]); cmd[2]="/forged"
     }
     return cmd
    })
    m:=newTensorFoldManager(t.TempDir(),tensorFoldRunnerFunc(func(_ context.Context,_ string,_ []string,name string,args ...string)([]byte,error){
     if strings.Contains(name+" "+strings.Join(args," "),"docker ps") {return []byte(id+"\n"),nil}; return data,nil
    }))
    r:=resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true),"running")
    err:=m.verifyRecipeContainer(context.Background(),r,worker)
    if (err==nil)!=(variant=="actual") {t.Fatalf("variant %s: %v",variant,err)}
   })
  }
 }
}
