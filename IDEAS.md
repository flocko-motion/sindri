# task and worker tiers 
tasks should be labelled by difficulty: junior/mid/senior 
workers should also come in variants: junior=haiku, mid=sonnet, senior=opus 
planner should always start with opus, user can change as needed in claude code 

assignment: a junior task should be assigned to a junior if one is running (not necessarily available!)... 
we rather wait for the junior to be avialble then waste tokens by assigning it to a senior.. 
if no worker of the required level is alive, a worker of the next higher level gets the assignement. 
but NOT the other way round: a senior task should never be given to a mid or junior. 

# more aggresive context clearing

context costs compute - we need to balance between enough context to gain from experience (project knowledge) 
but short enough to be cost effective... here's a formular for the sweetspot

pct(W) = P∞ + (P₀−P∞)·(W/W₀)^(−k), mit W₀=200k, P₀=37,5%, P∞=5%, k=0,6

200k → 75k (37,5%)
500k → 99k (19,7%)
1M → 131k (13,1%)
2M → 189k (9,4%)
4M → 297k (7,4%)
8M → 507k (6,3%)

Idea: whenever a worker had its final PR for a task merged and has no tasks left *in the hierarchy*, then 
automatic compacting check is executed: if the current context fill is above the threshold of the above formula, 
then we call compacttion before assigning the worker to the next hierarchy.  
