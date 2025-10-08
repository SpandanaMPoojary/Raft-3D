; Dummy G-code file
G28 ; Home all axes
G1 Z15.0 F9000 ; Move the platform down 15mm
G92 E0 ; Zero the extruder
G1 F140 E30 ; Extrude 30mm of filament
