from pathlib import Path
import json
root=Path.cwd(); original=root/'domain/patterns/ccgraph.go'; out=root/'.tmp-build/ccgraph-exhaustive-overlay.go'
source = original.read_text()
if source.count('func ccGraphGroups(pdgs ') != 1:
    raise SystemExit('Expected one ccGraphGroups entry point; review the benchmark overlay.')
out.parent.mkdir(parents=True, exist_ok=True)
s=source.replace('func ccGraphGroups(pdgs ', 'func ccFilteredGraphGroups(pdgs ',1)
s+='''
// Benchmark-only alternate routing. The production source stays filtered.
func ccGraphGroups(pdgs map[string]*MiningGraph, names map[string]string, astTypes map[string]map[string]int, neighborhood bool) [][]string {
 if neighborhood { return ccFilteredGraphGroups(pdgs,names,astTypes,true) }
 ids:=ccSortableIDs(pdgs)
 if len(ids)<2 { return nil }
 wls:=make([]*Wl,len(ids))
 for i,id:=range ids { wls[i]=NewWlLight(pdgs[id],nil) }
 parent:=ccMakeParent(ids)
 workers:=min(runtime.NumCPU(),len(ids))
 found:=make(chan []ccPairJob,workers)
 var wg sync.WaitGroup
 for worker:=range workers {
  wg.Add(1)
  go func(worker int) {
   defer wg.Done()
   ccAllMatchStripes(ids,wls,worker,workers,found)
  }(worker)
 }
 go func(){ wg.Wait();close(found) }()
 for pairs:=range found {
  for _,pair:=range pairs { union(parent,pair.a,pair.b) }
 }
 return groupsOfTwoOrMore(parent)
}

func ccAllMatchStripes(ids []string,wls []*Wl,worker,workers int,found chan<- []ccPairJob) {
 batch:=ccPairBatch{pairs:make([]ccPairJob,0,64),found:found}
 defer batch.flush()
 for i:=worker;i<len(ids);i+=workers {
  for j:=i+1;j<len(ids);j++ {
   if !similarityAtLeast(wls[i],wls[j],ccMatchThreshold) {continue}
   batch.add(ccPairJob{a:ids[i],b:ids[j]})
  }
 }
}
'''
out.write_text(s); (root/'.tmp-build/exhaustive-overlay.json').write_text(json.dumps({'Replace':{str(original):str(out)}}))
