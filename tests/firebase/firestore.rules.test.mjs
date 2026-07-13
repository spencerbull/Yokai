import { after, before, beforeEach, test } from "node:test"
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { fileURLToPath } from "node:url"

import {
  assertFails,
  assertSucceeds,
  initializeTestEnvironment,
} from "@firebase/rules-unit-testing"
import { deleteDoc, doc, getDoc, setDoc } from "firebase/firestore"

const projectId = "demo-yokai"
const rulesPath = fileURLToPath(new URL("../../firestore.rules", import.meta.url))
const validEnvelope = {
  version: 1,
  cipher: "aes-256-gcm",
  kdf: "argon2id-3-65536-4",
  salt: "AAAAAAAAAAAAAAAAAAAAAA",
  nonce: "AAAAAAAAAAAAAAAA",
  ciphertext: "encrypted-device-config",
}

let testEnvironment

before(async () => {
  testEnvironment = await initializeTestEnvironment({
    projectId,
    firestore: {
      rules: readFileSync(rulesPath, "utf8"),
      host: "127.0.0.1",
      port: 8080,
    },
  })
})

beforeEach(async () => {
  await testEnvironment.clearFirestore()
})

after(async () => {
  await testEnvironment.cleanup()
})

test("an authenticated user can create, read, and delete their encrypted config", async () => {
  const owner = testEnvironment.authenticatedContext("owner-user").firestore()
  const config = doc(owner, "device_configs", "owner-user")

  await assertSucceeds(setDoc(config, validEnvelope))
  const snapshot = await assertSucceeds(getDoc(config))
  assert.equal(snapshot.data().cipher, "aes-256-gcm")
  await assertSucceeds(deleteDoc(config))
})

test("another user cannot read, overwrite, or delete an owner's config", async () => {
  const owner = testEnvironment.authenticatedContext("owner-user").firestore()
  await assertSucceeds(setDoc(doc(owner, "device_configs", "owner-user"), validEnvelope))

  const intruder = testEnvironment.authenticatedContext("intruder-user").firestore()
  const target = doc(intruder, "device_configs", "owner-user")
  await assertFails(getDoc(target))
  await assertFails(setDoc(target, validEnvelope))
  await assertFails(deleteDoc(target))
})

test("unauthenticated clients cannot access cloud configs", async () => {
  const anonymous = testEnvironment.unauthenticatedContext().firestore()
  const target = doc(anonymous, "device_configs", "owner-user")
  await assertFails(getDoc(target))
  await assertFails(setDoc(target, validEnvelope))
})

test("rules reject plaintext, unexpected fields, and oversized ciphertext", async () => {
  const owner = testEnvironment.authenticatedContext("owner-user").firestore()
  const target = doc(owner, "device_configs", "owner-user")

  await assertFails(setDoc(target, { ...validEnvelope, cipher: "plaintext" }))
  await assertFails(setDoc(target, { ...validEnvelope, email: "leak@example.com" }))
  await assertFails(setDoc(target, { ...validEnvelope, ciphertext: "A".repeat(900001) }))
})

test("an authenticated user cannot write outside their UID document", async () => {
  const owner = testEnvironment.authenticatedContext("owner-user").firestore()
  await assertFails(setDoc(doc(owner, "device_configs", "different-user"), validEnvelope))
  await assertFails(setDoc(doc(owner, "other_collection", "owner-user"), validEnvelope))
})
