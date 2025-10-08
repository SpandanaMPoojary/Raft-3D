
Steps to run the project:

1.	**TERMINAL 1: **
2.	Start Node1 alone: Run it without --join:
3.	go run main.go --node-id=node1 --raft-port=12000 --http-port=8080
4.**	TERMINAL 2:**
5.	Then start Node2 with --join:
6.	go run main.go --node-id=node2 --raft-port=12001 --http-port=8081 --join=localhost:8080
7.	**TERMINAL 3**
8.	Then start Node3 similarly.
9.	go run main.go --node-id=node3 --raft-port=12002 --http-port=8082 --join=localhost:8080
10.**	TERMINAL 4:**
11.	curl http://localhost:8080/health
12.	You should now see:
13.	{"status":"ok","leader":"127.0.0.1:12000", ...}
14.	Then test with:
15.	curl http://localhost:8081/health
16.	curl http://localhost:8082/health
    
✅ 2. Add Printer (only on leader)
$headers = @{ "Content-Type" = "application/json" }
$body = @{
  id = "printer-test"
  company = "TestCorp"
  model = "T100"
} | ConvertTo-Json

Invoke-RestMethod -Uri "http://localhost:8080/api/v1/printers" -Method Post -Headers $headers -Body $body


✅ 3. View Printer from any node
curl http://localhost:8081/api/v1/printers
curl http://localhost:8082/api/v1/printers

You should see the same printer (printer-test) on all nodes 🖨️.

✅ 4. Try Adding a Filament
$body = @{
  id = "filament-test"
  type = "PLA"
  color = "Red"
  total_weight_in_grams = 1000
} | ConvertTo-Json

Invoke-RestMethod -Uri "http://localhost:8080/api/v1/filaments" -Method Post -Headers $headers -Body $body

Check from any node:
curl http://localhost:8082/api/v1/filaments

✅ 5. Restart Leader Node
•	Stop the leader (Ctrl+C)
•	Wait ~5-10 seconds
•	Check who the new leader is by re-running:

	curl http://localhost:8081/health
	curl http://localhost:8082/health

-------------------------------------------------

✅ 1. Create a Print Job:
$headers = @{ "Content-Type" = "application/json" }

$body = @{
  id = "job-001"
  printer_id = "printer-test"
  filament_id = "filament-test"
  filepath = "/models/spaceship.stl"
  print_weight_in_grams = 200
} | ConvertTo-Json

Invoke-RestMethod -Uri "http://localhost:8080/api/v1/print_jobs" -Method Post -Headers $headers -Body $body

{add port number of the leader node}

✅ 2. Fetch Print Jobs from any node

  curl http://localhost:8081/api/v1/print_jobs

You should see the job with status "Queued".

✅ 3. Update Job Status
Let’s say the job started running:
$body = @{ status = "Running" } | ConvertTo-Json
Invoke-RestMethod -Uri "http://localhost:8080/api/v1/print_jobs/job-001/status" -Method Post -Headers $headers -Body $body
{add port number of the leader node}

Then finish it:
$body = @{ status = "Done" } | ConvertTo-Json
Invoke-RestMethod -Uri "http://localhost:8080/api/v1/print_jobs/job-001/status" -Method Post -Headers $headers -Body $body

{add port number of the leader node}

✅ 4. Check Filament Weight Decrease
curl http://localhost:8082/api/v1/filaments/filament-test
remaining_weight_in_grams should now reflect the deduction (e.g., 1000 - 200 = 800)
