-- Disposable synthetic data. This is not source validation evidence.
INSERT INTO parcel_evidence_observations(artifact_id,observed_at,attempt_kind,manifest) VALUES ('vacancy-publication-test',now(),'full_scan','{}');
INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,parcel_number,year_built,sqft,shape)
VALUES ('union','179','union_county_gis','vacancy-test-a','vacancy-test-a',0,0,ST_MakeEnvelope(0,0,10,10,3857)),
('union','179','union_county_gis','vacancy-test-b','vacancy-test-b',1998,100,ST_MakeEnvelope(10,0,20,10,3857)),
('union','179','union_county_gis','vacancy-test-c','vacancy-test-c',0,0,ST_MakeEnvelope(20,0,30,10,3857));
INSERT INTO parcel_evidence_current(county_id,source_name,source_parcel_id,observation_id,source_hash,source_hash_version,normalized_hash,normalizer_version,normalized_hash_version,geometry_hash,geometry_hash_version,source_features,parcel_row_version)
SELECT 'union','union_county_gis',p.source_parcel_id,o.id,repeat('a',64),'test',repeat('b',64),'test','test',repeat('c',64),'test','[]'::jsonb,p.xmin::text
FROM parcels p CROSS JOIN parcel_evidence_observations o WHERE o.artifact_id='vacancy-publication-test' AND p.source_parcel_id IN ('vacancy-test-a','vacancy-test-b','vacancy-test-c');
CREATE TEMP TABLE vacancy_test_g(source_name text,id uuid,capture_id uuid) ON COMMIT DROP;
WITH c AS (INSERT INTO ingest_captures(source_name,observed_at,manifest,manifest_sha256,storage_mode,published_at)
VALUES ('union_structures_171',now(),convert_to('{"source":"union_structures_171","kind":"full_scan","count":1,"receipts":[{"http_status":200,"response_complete":true,"request":"https://example.test/171/query?where=1%3D1"}]}','UTF8'),decode(repeat('a',64),'hex'),'local',now()) RETURNING id,source_name,observed_at),g AS (
INSERT INTO vacancy_live_generations(source_name,capture_id,retrieved_at,observed_count,validation_note,normalized_sha256,normalizer_version)
SELECT source_name,id,observed_at,1,'synthetic test only',repeat('d',64),'vacancy-rows-v1' FROM c RETURNING id,source_name,capture_id)
INSERT INTO vacancy_test_g SELECT source_name,id,capture_id FROM g;
WITH c AS (INSERT INTO ingest_captures(source_name,observed_at,manifest,manifest_sha256,storage_mode,published_at)
VALUES ('union_assessed_value_0',now(),convert_to('{"source":"union_assessed_value_0","kind":"full_scan","count":4,"receipts":[{"http_status":200,"response_complete":true,"request":"https://example.test/0/query?where=1%3D1"}]}','UTF8'),decode(repeat('b',64),'hex'),'local',now()) RETURNING id,source_name,observed_at),g AS (
INSERT INTO vacancy_live_generations(source_name,capture_id,retrieved_at,observed_count,validation_note,normalized_sha256,normalizer_version)
SELECT source_name,id,observed_at,4,'synthetic test only',repeat('e',64),'vacancy-rows-v1' FROM c RETURNING id,source_name,capture_id)
INSERT INTO vacancy_test_g SELECT source_name,id,capture_id FROM g;
INSERT INTO vacancy_latest_observations(source_name,capture_id,observed_at) SELECT g.source_name,g.capture_id,c.observed_at FROM vacancy_test_g g JOIN ingest_captures c ON c.id=g.capture_id
ON CONFLICT(source_name) DO UPDATE SET capture_id=EXCLUDED.capture_id,observed_at=EXCLUDED.observed_at;
INSERT INTO vacancy_live_validation(generation_id,capture_id,source_name,normalized_sha256,source_url,terms_evidence,coverage_evidence,count_evidence,snapshot_evidence,source_as_of,status_evidence,current_statuses)
SELECT id,capture_id,source_name,repeat('d',64),'https://example.test/171/query','synthetic terms','synthetic county','synthetic count','synthetic repeat',current_date,'test existing status',ARRAY['existing'] FROM vacancy_test_g WHERE source_name='union_structures_171';
INSERT INTO vacancy_live_validation(generation_id,capture_id,source_name,normalized_sha256,source_url,terms_evidence,coverage_evidence,count_evidence,snapshot_evidence,source_as_of,effective_av_year)
SELECT id,capture_id,source_name,repeat('e',64),'https://example.test/0/query','synthetic terms','synthetic county','synthetic count of three','synthetic repeat',current_date,2026 FROM vacancy_test_g WHERE source_name='union_assessed_value_0';
INSERT INTO vacancy_live_assessments(generation_id,ordinal,object_id,parcel_number,parcel_year,assess_category,market_building)
SELECT id,1,'1','vacancy-test-a',2026,'AV',0 FROM vacancy_test_g WHERE source_name='union_assessed_value_0';
INSERT INTO vacancy_live_assessments(generation_id,ordinal,object_id,parcel_number,parcel_year,assess_category,market_building)
SELECT id,2,'2','vacancy-test-b',2026,'AV',100 FROM vacancy_test_g WHERE source_name='union_assessed_value_0';
INSERT INTO vacancy_live_assessments(generation_id,ordinal,object_id,parcel_number,parcel_year,assess_category,market_building)
SELECT id,3,'1','vacancy-test-a',2026,'AV',0 FROM vacancy_test_g WHERE source_name='union_assessed_value_0';
INSERT INTO vacancy_live_assessments(generation_id,ordinal,object_id,parcel_number,parcel_year,assess_category,market_building)
SELECT id,4,'4','vacancy-test-c',2026,'AV',100 FROM vacancy_test_g WHERE source_name='union_assessed_value_0';
INSERT INTO vacancy_live_structures(generation_id,ordinal,object_id,pin,status,source_crs,geometry)
SELECT id,1,'1','vacancy-test-b','existing',3857,'{"rings":[[[10,1],[15,1],[15,5],[10,5],[10,1]]]}'::jsonb FROM vacancy_test_g WHERE source_name='union_structures_171';
