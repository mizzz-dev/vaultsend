-- file は upload session 作成時に shipment へ所属するため、作成後の付け替えを禁止する。
-- 別ユーザー・別組織の draft shipment へ file を混在させる事故と認可漏れをDB境界でも防ぐ。
CREATE OR REPLACE FUNCTION prevent_file_shipment_reassignment()
RETURNS trigger AS $$
BEGIN
    IF NEW.shipment_id IS DISTINCT FROM OLD.shipment_id THEN
        RAISE EXCEPTION 'files.shipment_id is immutable'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_files_prevent_shipment_reassignment
    BEFORE UPDATE OF shipment_id ON files
    FOR EACH ROW
    EXECUTE FUNCTION prevent_file_shipment_reassignment();
